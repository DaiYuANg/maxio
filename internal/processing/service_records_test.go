package processing

import (
	"context"
	"errors"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestRecordKeepsObjectKeyWhitespaceDistinct(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
	)
	ctx := context.Background()
	spaced := ObjectRef{Bucket: "docs", Key: " file.txt", VersionID: "v1"}
	plain := ObjectRef{Bucket: "docs", Key: "file.txt", VersionID: "v1"}

	service.ProcessAfterCommit(ctx, Input{Object: spaced})

	if _, found := service.Record(ctx, plain); found {
		t.Fatal("did not expect plain key to match spaced key")
	}
	if _, found := service.Record(ctx, spaced); !found {
		t.Fatal("expected spaced key processing record")
	}
}

type failingRecordStore struct {
	err error
}

func (s failingRecordStore) UpsertProcessingRecord(context.Context, model.ProcessingRecord) (model.ProcessingRecord, error) {
	return model.ProcessingRecord{}, s.err
}

func (s failingRecordStore) GetProcessingRecord(context.Context, string, string, string, string) (model.ProcessingRecord, bool, error) {
	return model.ProcessingRecord{}, false, s.err
}

func (s failingRecordStore) ListProcessingRecords(context.Context, string, int) (*collectionlist.List[model.ProcessingRecord], error) {
	return nil, s.err
}

func (s failingRecordStore) DeleteProcessingRecord(context.Context, string, string, string, string) (bool, error) {
	return false, s.err
}

func TestAsyncStrictReadFailsClosedWhenRecordStoreFails(t *testing.T) {
	service := NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncStrict, Timeout: time.Second},
		failingRecordStore{err: errors.New("metadata offline")},
	)
	err := service.EnsureReadAllowed(context.Background(), ObjectRef{Bucket: "docs", Key: "doc.txt", VersionID: "v1"})
	if err == nil {
		t.Fatal("expected read to fail when processing record store fails")
	}
}

func TestAsyncStrictReadHonorsFailOpenWhenRecordStoreFails(t *testing.T) {
	service := NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncStrict, Timeout: time.Second, FailOpen: true},
		failingRecordStore{err: errors.New("metadata offline")},
	)
	err := service.EnsureReadAllowed(context.Background(), ObjectRef{Bucket: "docs", Key: "doc.txt", VersionID: "v1"})
	if err != nil {
		t.Fatalf("EnsureReadAllowed error = %v, want nil", err)
	}
}

func TestInlineStrictPromotesDigestRecordAfterCommit(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		testProcessor{status: StatusSucceeded},
	)
	ctx := context.Background()
	preflight := ObjectRef{Bucket: "docs", Key: "doc.txt", Digest: "sha256:abc"}
	if err := service.ProcessBeforeCommit(ctx, Input{Object: preflight}); err != nil {
		t.Fatalf("ProcessBeforeCommit error: %v", err)
	}
	final := ObjectRef{Bucket: "docs", Key: "doc.txt", VersionID: "v1", Digest: "sha256:abc"}
	service.ProcessAfterCommit(ctx, Input{Object: final})

	record, found := service.Record(ctx, final)
	if !found {
		t.Fatal("expected promoted version processing record")
	}
	if record.Status != StatusSucceeded {
		t.Fatalf("status = %q, want %q", record.Status, StatusSucceeded)
	}
	if record.Results == nil || record.Results.Len() != 1 {
		t.Fatalf("results = %#v, want one promoted result", record.Results)
	}
}

func TestInlineStrictRemovesDigestRecordAfterPromotion(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		testProcessor{status: StatusSucceeded},
	)
	ctx := context.Background()
	preflight := ObjectRef{Bucket: "docs", Key: "doc.txt", Digest: "sha256:cleanup"}
	if err := service.ProcessBeforeCommit(ctx, Input{Object: preflight}); err != nil {
		t.Fatalf("ProcessBeforeCommit error: %v", err)
	}
	final := ObjectRef{Bucket: "docs", Key: "doc.txt", VersionID: "v1", Digest: "sha256:cleanup"}
	service.ProcessAfterCommit(ctx, Input{Object: final})

	if _, found := service.Record(ctx, preflight); found {
		t.Fatal("did not expect digest-only processing record after promotion")
	}
	if _, found := service.Record(ctx, final); !found {
		t.Fatal("expected final version processing record after promotion")
	}
}

func TestCountLimitedReportsTruncation(t *testing.T) {
	count, truncated, err := countLimited(strings.NewReader("hello"), 4)
	if err != nil {
		t.Fatalf("countLimited error: %v", err)
	}
	if count != 4 {
		t.Fatalf("count = %d, want 4", count)
	}
	if !truncated {
		t.Fatal("expected truncated response")
	}
}

type shortWriter struct {
	data []byte
}

func (w *shortWriter) Write(data []byte) (int, error) {
	if len(data) > 1 {
		data = data[:1]
	}
	w.data = append(w.data, data...)
	return len(data), nil
}

func TestWriteFullHandlesShortWrites(t *testing.T) {
	writer := &shortWriter{}
	if err := writeFull(writer, []byte("hello")); err != nil {
		t.Fatalf("writeFull error: %v", err)
	}
	if string(writer.data) != "hello" {
		t.Fatalf("written = %q, want hello", string(writer.data))
	}
}

func TestSnapshotReportsProcessorCapabilities(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		testProcessor{name: "policy", status: StatusSucceeded},
	)
	snapshot := service.Snapshot()
	if snapshot.Capabilities == nil {
		t.Fatal("expected capabilities list")
	}
	capabilities := snapshot.Capabilities.Values()
	if len(capabilities) != 1 || capabilities[0] != string(CapabilityPolicyEvaluation) {
		t.Fatalf("capabilities = %#v, want policy_evaluation", capabilities)
	}
}

func TestClamAVFailureResultReportsReasonAndAddress(t *testing.T) {
	processor := NewClamAVProcessor(ClamAVConfig{Address: "127.0.0.1:3310", Timeout: time.Second})
	result, err := processor.failureResult("connect", errors.New("connection refused"))
	if err == nil {
		t.Fatal("expected failure error")
	}
	if result.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", result.Status, StatusFailed)
	}
	if result.Metadata["reason"] != "connect" {
		t.Fatalf("reason = %q, want connect", result.Metadata["reason"])
	}
	if result.Metadata["address"] != "127.0.0.1:3310" {
		t.Fatalf("address = %q, want 127.0.0.1:3310", result.Metadata["address"])
	}
}

func TestClamAVResponseMetadataReportsUnknownVerdict(t *testing.T) {
	metadata := clamAVResponseMetadata("stream: service overloaded")
	if metadata["verdict"] != "unknown" {
		t.Fatalf("verdict = %q, want unknown", metadata["verdict"])
	}
}

func TestReadClamAVResponseRejectsOversizedResponse(t *testing.T) {
	_, err := readClamAVResponse(strings.NewReader(strings.Repeat("x", int(defaultClamAVResponseMaxBytes)+1)))
	if err == nil {
		t.Fatal("expected oversized clamav response error")
	}
}

func TestRunStoresFailedWhenEveryStrictProcessorSkips(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		testProcessor{name: "skip-a", status: StatusSkipped},
		testProcessor{name: "skip-b", status: StatusSkipped},
	)
	object := ObjectRef{Bucket: "docs", Key: "skipped.txt", VersionID: "v1"}
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
