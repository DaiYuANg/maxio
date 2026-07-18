package processing

import (
	"context"
	"fmt"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"log/slog"
	"testing"
	"time"
)

func (p blockingTestProcessor) Process(ctx context.Context, _ Input) (ProcessorResult, error) {
	if p.finished != nil {
		defer close(p.finished)
	}
	if p.started != nil {
		close(p.started)
	}
	if p.release != nil {
		select {
		case <-p.release:
		case <-ctx.Done():
			return ProcessorResult{Processor: p.name, Status: StatusFailed}, fmt.Errorf("processor context canceled: %w", ctx.Err())
		}
	}
	return ProcessorResult{Processor: p.name, Status: StatusSucceeded}, nil
}

func TestInlineStrictPromotionKeepsResultWhenAsyncProcessorQueued(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeInlineStrict),
		BindProcessor(blockingTestProcessor{name: "tika", started: started, release: release}, ModeAsyncPermissive),
	)
	ctx := context.Background()
	preflight := ObjectRef{Bucket: "docs", Key: "mixed-stage.txt", Digest: "sha256:mixed"}
	if err := service.ProcessBeforeCommit(ctx, Input{Object: preflight}); err != nil {
		t.Fatalf("preflight processing: %v", err)
	}
	final := preflight
	final.VersionID = "v1"
	service.ProcessAfterCommit(ctx, Input{Object: final})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("async processor did not start")
	}
	defer close(release)

	record, found := service.Record(ctx, final)
	if !found {
		t.Fatal("expected promoted final processing record")
	}
	if _, noopFound := findProcessorResult(record.Results, "noop", ModeInlineStrict); !noopFound {
		t.Fatalf("results = %#v, want promoted inline noop result", record.Results)
	}
	async, found := findProcessorResult(record.Results, "tika", ModeAsyncPermissive)
	if !found {
		t.Fatalf("results = %#v, want async tika result", record.Results)
	}
	if async.Status != StatusQueued && async.Status != StatusRunning {
		t.Fatalf("async status = %q, want queued or running", async.Status)
	}
	if _, found := service.Record(ctx, preflight); found {
		t.Fatal("expected digest-only preflight record to be removed after promotion")
	}
}

func findProcessorResult(results *collectionlist.List[ProcessorResult], processor, mode string) (ProcessorResult, bool) {
	if results == nil {
		return ProcessorResult{}, false
	}
	var found ProcessorResult
	ok := false
	results.Range(func(_ int, result ProcessorResult) bool {
		if result.Processor == processor && result.Mode == mode {
			found = result
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func TestNewServiceBackfillsProcessorResultMode(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		NewNoopProcessor(),
	)
	object := ObjectRef{Bucket: "docs", Key: "mode.txt", VersionID: "v1"}
	if err := service.ProcessBeforeCommit(context.Background(), Input{Object: object}); err != nil {
		t.Fatalf("process before commit: %v", err)
	}
	record, found := service.Record(context.Background(), object)
	if !found {
		t.Fatal("expected processing record")
	}
	result, found := findProcessorResult(record.Results, "noop", ModeInlineStrict)
	if !found {
		t.Fatalf("results = %#v, want noop inline result", record.Results)
	}
	if result.Mode != ModeInlineStrict {
		t.Fatalf("result mode = %q, want %q", result.Mode, ModeInlineStrict)
	}
}

func TestReadDecisionUsesLegacyModelessResultOnlyWhenRecordModeMatches(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
	)
	record := Record{
		Object: ObjectRef{Bucket: "docs", Key: "legacy.txt", VersionID: "v1"},
		Mode:   ModeInlineStrict,
		Status: StatusSucceeded,
		Results: collectionlist.NewList(
			ProcessorResult{Processor: "clamav", Status: StatusSucceeded},
		),
	}
	decision := service.ReadDecision(record)
	if !decision.Allowed {
		t.Fatalf("read decision = %#v, want legacy inline result allowed", decision)
	}
}

func TestReadDecisionRejectsLegacyModelessResultWhenRecordModeDiffers(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
	)
	record := Record{
		Object: ObjectRef{Bucket: "docs", Key: "legacy.txt", VersionID: "v1"},
		Mode:   ModeAsyncPermissive,
		Status: StatusSucceeded,
		Results: collectionlist.NewList(
			ProcessorResult{Processor: "clamav", Status: StatusSucceeded},
		),
	}
	decision := service.ReadDecision(record)
	if decision.Allowed || decision.Reason != "pending" {
		t.Fatalf("read decision = %#v, want pending block", decision)
	}
}

func TestReadDecisionRejectsDifferentModeResultEvenWhenRecordModeMatches(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
	)
	record := Record{
		Object: ObjectRef{Bucket: "docs", Key: "strict-mode.txt", VersionID: "v1"},
		Mode:   ModeInlineStrict,
		Status: StatusSucceeded,
		Results: collectionlist.NewList(
			ProcessorResult{Processor: "clamav", Mode: ModeAsyncPermissive, Status: StatusSucceeded},
		),
	}
	decision := service.ReadDecision(record)
	if decision.Allowed || decision.Reason != "pending" {
		t.Fatalf("read decision = %#v, want pending block", decision)
	}
}

func TestProcessAfterCommitDoesNotOverwriteExistingEmptyRecord(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeInlineStrict),
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "existing-empty.txt", VersionID: "v1"}
	if err := service.storeRecord(ctx, object, StatusFailed, "existing failure", collectionlist.NewList[ProcessorResult]()); err != nil {
		t.Fatalf("store existing record: %v", err)
	}
	service.ProcessAfterCommit(ctx, Input{Object: object})
	record, found := service.Record(ctx, object)
	if !found {
		t.Fatal("expected existing processing record")
	}
	if record.Status != StatusFailed {
		t.Fatalf("status = %q, want existing failed status", record.Status)
	}
	if record.Error != "existing failure" {
		t.Fatalf("error = %q, want existing failure", record.Error)
	}
}

func TestDiscardPreventsAsyncProcessorFromRecreatingVersionRecord(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(blockingTestProcessor{name: "tika", started: started, release: release, finished: finished}, ModeAsyncPermissive),
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "deleted-while-processing.txt", VersionID: "v1"}
	service.ProcessAfterCommit(ctx, Input{Object: object})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("async processor did not start")
	}
	service.Discard(ctx, object)
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("async processor did not finish")
	}
	if _, found := service.Record(ctx, object); found {
		t.Fatal("expected discarded version processing record not to be recreated")
	}
}
