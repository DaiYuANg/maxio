package processing

import (
	"context"
	"errors"
	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestInlineStrictFailsClosedOnProcessingRecordWriteError(t *testing.T) {
	service := NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		failingRecordStore{err: errors.New("metadata offline")},
		NewNoopProcessor(),
	)
	err := service.ProcessBeforeCommit(context.Background(), Input{Object: ObjectRef{Bucket: "docs", Key: "record-store.txt", VersionID: "v1"}})
	if err == nil {
		t.Fatal("expected inline strict processing to fail when processing record persistence fails")
	}
	if !strings.Contains(err.Error(), "metadata offline") {
		t.Fatalf("error = %v, want metadata offline", err)
	}
}

func TestInlineStrictFailOpenAllowsProcessingRecordWriteError(t *testing.T) {
	service := NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second, FailOpen: true},
		failingRecordStore{err: errors.New("metadata offline")},
		NewNoopProcessor(),
	)
	if err := service.ProcessBeforeCommit(context.Background(), Input{Object: ObjectRef{Bucket: "docs", Key: "record-store.txt", VersionID: "v1"}}); err != nil {
		t.Fatalf("expected fail-open inline strict processing to allow record store failure: %v", err)
	}
}

func TestAsyncPermissiveProcessorPendingDoesNotBlockStrictReadGate(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
		BindProcessor(testProcessor{name: "tika", status: StatusSucceeded}, ModeAsyncPermissive),
	)
	object := ObjectRef{Bucket: "docs", Key: "mixed-stage.txt", VersionID: "v1"}
	results := collectionlist.NewList(
		ProcessorResult{Processor: "clamav", Mode: ModeInlineStrict, Status: StatusSucceeded},
		ProcessorResult{Processor: "tika", Mode: ModeAsyncPermissive, Status: StatusQueued},
	)
	if err := service.storeRecord(context.Background(), object, StatusQueued, "", results); err != nil {
		t.Fatalf("store processing record: %v", err)
	}
	if err := service.EnsureReadAllowed(context.Background(), object); err != nil {
		t.Fatalf("EnsureReadAllowed error = %v, want nil", err)
	}
}

func TestAsyncStrictProcessorPendingBlocksStrictReadGate(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "classifier", status: StatusSucceeded}, ModeAsyncStrict),
	)
	object := ObjectRef{Bucket: "docs", Key: "strict-stage.txt", VersionID: "v1"}
	results := collectionlist.NewList(
		ProcessorResult{Processor: "classifier", Mode: ModeAsyncStrict, Status: StatusQueued},
	)
	if err := service.storeRecord(context.Background(), object, StatusQueued, "", results); err != nil {
		t.Fatalf("store processing record: %v", err)
	}
	err := service.EnsureReadAllowed(context.Background(), object)
	if !errors.Is(err, ErrProcessingPending) {
		t.Fatalf("EnsureReadAllowed error = %v, want ErrProcessingPending", err)
	}
}

func TestProcessorBindingSnapshotReportsModes(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
		BindProcessor(testProcessor{name: "tika", status: StatusSucceeded}, ModeAsyncPermissive),
	)
	snapshot := service.Snapshot()
	if snapshot.ProcessorModes["clamav"] != ModeInlineStrict {
		t.Fatalf("clamav mode = %q, want %q", snapshot.ProcessorModes["clamav"], ModeInlineStrict)
	}
	if snapshot.ProcessorModes["tika"] != ModeAsyncPermissive {
		t.Fatalf("tika mode = %q, want %q", snapshot.ProcessorModes["tika"], ModeAsyncPermissive)
	}
}

func TestProcessorBindingSnapshotReportsProcessorFailOpen(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(NewTikaProcessor(TikaConfig{FailOpen: true}), ModeAsyncPermissive),
	)
	snapshot := service.Snapshot()
	if !snapshot.ProcessorFailOpen["tika"] {
		t.Fatalf("tika fail-open = %v, want true", snapshot.ProcessorFailOpen["tika"])
	}
}

func TestAsyncProcessorRecordWriteErrorDoesNotInheritInlineStrict(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		failingRecordStore{err: errors.New("metadata offline")},
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
		BindProcessor(testProcessor{name: "tika", status: StatusSucceeded}, ModeAsyncPermissive),
	)
	err := service.run(
		context.Background(),
		Input{Object: ObjectRef{Bucket: "docs", Key: "async-store-error.txt", VersionID: "v1"}},
		service.bindingsForModes(ModeAsyncPermissive),
		nil,
	)
	if err != nil {
		t.Fatalf("async processing inherited inline strict store failure: %v", err)
	}
}

func TestAsyncProcessingContinuesAfterProcessorFailure(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "extract", status: StatusFailed, err: errors.New("extract failed")}, ModeAsyncPermissive),
		BindProcessor(testProcessor{name: "classify", status: StatusSucceeded}, ModeAsyncPermissive),
	)
	object := ObjectRef{Bucket: "docs", Key: "continue-after-failure.txt", VersionID: "v1"}
	err := service.run(context.Background(), Input{Object: object}, service.bindingsForModes(ModeAsyncPermissive), nil)
	if !errors.Is(err, ErrProcessingFailed) {
		t.Fatalf("run error = %v, want ErrProcessingFailed", err)
	}
	record, found := service.Record(context.Background(), object)
	if !found {
		t.Fatal("expected processing record")
	}
	if _, found := findProcessorResult(record.Results, "extract", ModeAsyncPermissive); !found {
		t.Fatalf("results = %#v, want failed extract result", record.Results)
	}
	if _, found := findProcessorResult(record.Results, "classify", ModeAsyncPermissive); !found {
		t.Fatalf("results = %#v, want successful classify result", record.Results)
	}
}

func TestStrictReadGateRequiresConfiguredStrictProcessorResult(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
		BindProcessor(testProcessor{name: "tika", status: StatusSucceeded}, ModeAsyncPermissive),
	)
	object := ObjectRef{Bucket: "docs", Key: "missing-strict.txt", VersionID: "v1"}
	results := collectionlist.NewList(
		ProcessorResult{Processor: "tika", Mode: ModeAsyncPermissive, Status: StatusSucceeded},
	)
	if err := service.storeRecord(context.Background(), object, StatusSucceeded, "", results); err != nil {
		t.Fatalf("store processing record: %v", err)
	}
	err := service.EnsureReadAllowed(context.Background(), object)
	if !errors.Is(err, ErrProcessingPending) {
		t.Fatalf("EnsureReadAllowed error = %v, want ErrProcessingPending", err)
	}
}

func TestReadDecisionReportsPermissivePendingAsAllowed(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
		BindProcessor(testProcessor{name: "tika", status: StatusSucceeded}, ModeAsyncPermissive),
	)
	record := Record{
		Object: ObjectRef{Bucket: "docs", Key: "read-decision.txt", VersionID: "v1"},
		Status: StatusQueued,
		Results: collectionlist.NewList(
			ProcessorResult{Processor: "clamav", Mode: ModeInlineStrict, Status: StatusSucceeded},
			ProcessorResult{Processor: "tika", Mode: ModeAsyncPermissive, Status: StatusQueued},
		),
	}
	decision := service.ReadDecision(record)
	if !decision.Allowed {
		t.Fatalf("read decision = %#v, want allowed", decision)
	}
}

func TestReadDecisionReportsMissingStrictResultAsPending(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
	)
	record := Record{Object: ObjectRef{Bucket: "docs", Key: "read-decision.txt", VersionID: "v1"}, Status: StatusSucceeded}
	decision := service.ReadDecision(record)
	if decision.Allowed || decision.Reason != "pending" {
		t.Fatalf("read decision = %#v, want pending block", decision)
	}
}

type blockingTestProcessor struct {
	name     string
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (p blockingTestProcessor) Name() string {
	return p.name
}

func (p blockingTestProcessor) Capabilities() *collectionset.Set[Capability] {
	return collectionset.NewSet[Capability]()
}
