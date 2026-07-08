package processing

import (
	"context"
	"errors"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
	"log/slog"
	"testing"
	"time"
)

func TestListRecordsOverFetchesToReplaceDiscardedStoreRecords(t *testing.T) {
	ctx := context.Background()
	discarded := ObjectRef{Bucket: "docs", Key: "discarded-list-limit.txt", VersionID: "v1"}
	live := ObjectRef{Bucket: "docs", Key: "live-list-limit.txt", VersionID: "v1"}
	storeRecords := collectionlist.NewList(
		model.ProcessingRecord{Bucket: discarded.Bucket, Key: discarded.Key, VersionID: discarded.VersionID, Mode: ModeAsyncPermissive, Status: StatusSucceeded},
		model.ProcessingRecord{Bucket: live.Bucket, Key: live.Key, VersionID: live.VersionID, Mode: ModeAsyncPermissive, Status: StatusSucceeded},
	)
	service := NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{records: storeRecords},
		NewNoopProcessor(),
	)
	service.Discard(ctx, discarded)
	records, err := service.ListRecords(ctx, StatusSucceeded, 1)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if records.Len() != 1 {
		t.Fatalf("records len = %d, want 1", records.Len())
	}
	if records.Values()[0].Object.Key != live.Key {
		t.Fatalf("record key = %q, want live record", records.Values()[0].Object.Key)
	}
}

func TestDisabledSnapshotHidesProcessorBindings(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: false, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "clamav", status: StatusSucceeded}, ModeInlineStrict),
	)
	snapshot := service.Snapshot()
	if snapshot.Mode != ModeDisabled {
		t.Fatalf("mode = %q, want %q", snapshot.Mode, ModeDisabled)
	}
	if snapshot.Processors.Len() != 0 {
		t.Fatalf("processors = %#v, want empty", snapshot.Processors.Values())
	}
	if len(snapshot.ProcessorModes) != 0 {
		t.Fatalf("processor modes = %#v, want empty", snapshot.ProcessorModes)
	}
	if snapshot.Capabilities.Len() != 0 {
		t.Fatalf("capabilities = %#v, want empty", snapshot.Capabilities.Values())
	}
}

func TestInlineStrictRejectsSkippedProcessor(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		testProcessor{name: "tika", status: StatusSkipped},
	)
	object := ObjectRef{Bucket: "docs", Key: "strict-skipped.txt", VersionID: "v1"}
	err := service.ProcessBeforeCommit(context.Background(), Input{Object: object})
	if !errors.Is(err, ErrProcessingFailed) {
		t.Fatalf("ProcessBeforeCommit error = %v, want ErrProcessingFailed", err)
	}
	record, found := service.Record(context.Background(), object)
	if !found {
		t.Fatal("expected processing record")
	}
	if record.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", record.Status, StatusFailed)
	}
}

func TestAsyncStrictReadRejectsSkippedProcessorResult(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(testProcessor{name: "tika", status: StatusSkipped}, ModeAsyncStrict),
	)
	object := ObjectRef{Bucket: "docs", Key: "strict-skipped-read.txt", VersionID: "v1"}
	results := collectionlist.NewList(ProcessorResult{Processor: "tika", Mode: ModeAsyncStrict, Status: StatusSkipped})
	if err := service.storeRecord(context.Background(), object, StatusSkipped, "", results); err != nil {
		t.Fatalf("store processing record: %v", err)
	}
	err := service.EnsureReadAllowed(context.Background(), object)
	if !errors.Is(err, ErrProcessingFailed) {
		t.Fatalf("EnsureReadAllowed error = %v, want ErrProcessingFailed", err)
	}
}
