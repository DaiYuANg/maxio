package processing

import (
	"context"
	"errors"
	collectionset "github.com/arcgolabs/collectionx/set"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunStoresSucceededWhenStrictProcessorSucceeds(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		testProcessor{name: "success", status: StatusSucceeded},
	)
	object := ObjectRef{Bucket: "docs", Key: "succeeded.txt", VersionID: "v1"}
	if err := service.ProcessBeforeCommit(context.Background(), Input{Object: object}); err != nil {
		t.Fatalf("ProcessBeforeCommit error: %v", err)
	}
	record, found := service.Record(context.Background(), object)
	if !found {
		t.Fatal("expected processing record")
	}
	if record.Status != StatusSucceeded {
		t.Fatalf("status = %q, want %q", record.Status, StatusSucceeded)
	}
}

func TestRunRejectsUnknownProcessorStatus(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		testProcessor{name: "bad-status", status: "mystery"},
	)
	object := ObjectRef{Bucket: "docs", Key: "bad-status.txt", VersionID: "v1"}
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

func TestTikaProcessorFailOpenSkipsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	processor := NewTikaProcessor(TikaConfig{URL: server.URL, Timeout: time.Second, MaxBytes: 1024, FailOpen: true})
	result, err := processor.Process(context.Background(), Input{
		Object: ObjectRef{Bucket: "docs", Key: "doc.txt", VersionID: "v1", Size: int64(len("hello")), ContentType: "text/plain"},
		OpenContent: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("hello")), nil
		},
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if result.Status != StatusSkipped {
		t.Fatalf("status = %q, want %q", result.Status, StatusSkipped)
	}
	if result.Metadata["fail_open"] != "true" {
		t.Fatalf("fail_open metadata = %q, want true", result.Metadata["fail_open"])
	}
	if result.Metadata["reason"] != "bad status" {
		t.Fatalf("reason = %q, want bad status", result.Metadata["reason"])
	}
	if result.Metadata["endpoint"] != tikaEndpoint(server.URL) {
		t.Fatalf("endpoint = %q, want %q", result.Metadata["endpoint"], tikaEndpoint(server.URL))
	}
}

func TestTikaProcessorFailClosedReturnsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	processor := NewTikaProcessor(TikaConfig{URL: server.URL, Timeout: time.Second, MaxBytes: 1024})
	result, err := processor.Process(context.Background(), Input{
		Object: ObjectRef{Bucket: "docs", Key: "doc.txt", VersionID: "v1", Size: int64(len("hello")), ContentType: "text/plain"},
		OpenContent: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("hello")), nil
		},
	})
	if err == nil {
		t.Fatal("expected Process error")
	}
	if result.Status != StatusFailed {
		t.Fatalf("status = %q, want %q", result.Status, StatusFailed)
	}
	if result.Metadata["fail_open"] != "false" {
		t.Fatalf("fail_open metadata = %q, want false", result.Metadata["fail_open"])
	}
	if result.Metadata["reason"] != "bad status" {
		t.Fatalf("reason = %q, want bad status", result.Metadata["reason"])
	}
	if result.Metadata["endpoint"] != tikaEndpoint(server.URL) {
		t.Fatalf("endpoint = %q, want %q", result.Metadata["endpoint"], tikaEndpoint(server.URL))
	}
}

type capabilityOnlyProcessor struct {
	name         string
	capabilities []Capability
}

func (p capabilityOnlyProcessor) Name() string {
	return p.name
}

func (p capabilityOnlyProcessor) Capabilities() *collectionset.Set[Capability] {
	return collectionset.NewSet(p.capabilities...)
}

func (p capabilityOnlyProcessor) Process(context.Context, Input) (ProcessorResult, error) {
	return ProcessorResult{Processor: p.name, Status: StatusSucceeded}, nil
}

func TestSnapshotReportsSortedCapabilities(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		capabilityOnlyProcessor{name: "mixed", capabilities: []Capability{CapabilityTextExtraction, CapabilityAntivirus}},
	)
	snapshot := service.Snapshot()
	capabilities := snapshot.Capabilities.Values()
	expected := []string{string(CapabilityAntivirus), string(CapabilityTextExtraction)}
	if len(capabilities) != len(expected) {
		t.Fatalf("capabilities = %#v, want %#v", capabilities, expected)
	}
	for i := range expected {
		if capabilities[i] != expected[i] {
			t.Fatalf("capabilities = %#v, want %#v", capabilities, expected)
		}
	}
}

func TestListRecordsNormalizesStatusFilter(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		testProcessor{status: StatusSucceeded},
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "normalized-status.txt", VersionID: "v1"}
	if err := service.ProcessBeforeCommit(ctx, Input{Object: object}); err != nil {
		t.Fatalf("ProcessBeforeCommit error: %v", err)
	}
	records, err := service.ListRecords(ctx, " Succeeded ", 10)
	if err != nil {
		t.Fatalf("ListRecords error: %v", err)
	}
	if records.Len() != 1 {
		t.Fatalf("records len = %d, want 1", records.Len())
	}
}

func TestReadTikaRequestBodyAllowsExactLimit(t *testing.T) {
	data, oversized, err := readTikaRequestBody(strings.NewReader("1234"), 4)
	if err != nil {
		t.Fatalf("read exact limit: %v", err)
	}
	if oversized {
		t.Fatal("oversized = true, want false")
	}
	if string(data) != "1234" {
		t.Fatalf("data = %q, want exact body", string(data))
	}
}

func TestReadTikaRequestBodyRejectsOverflow(t *testing.T) {
	_, oversized, err := readTikaRequestBody(strings.NewReader("12345"), 4)
	if err != nil {
		t.Fatalf("read overflow body: %v", err)
	}
	if !oversized {
		t.Fatal("oversized = false, want true")
	}
}

func TestReadTikaRMetaSummarizesMetadataWithoutTextPayload(t *testing.T) {
	metadata, err := readTikaRMeta(strings.NewReader(`[{"X-TIKA:content":"hello","Content-Type":"text/plain","dc:creator":["alice"],"X-TIKA:Exception:write_limit_reached":"true"}]`), 1024)
	if err != nil {
		t.Fatalf("read tika metadata: %v", err)
	}
	if metadata["text_bytes"] != "5" {
		t.Fatalf("text_bytes = %q, want 5", metadata["text_bytes"])
	}
	if metadata["text_truncated"] != "true" {
		t.Fatalf("text_truncated = %q, want true", metadata["text_truncated"])
	}
	if metadata["author"] != "alice" {
		t.Fatalf("author = %q, want alice", metadata["author"])
	}
	if _, exposed := metadata["X-TIKA:content"]; exposed {
		t.Fatal("did not expect extracted text payload in processor metadata")
	}
}

func TestTikaProcessorSkipsUnknownSizeObjectWhenContentExceedsLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			t.Fatalf("discard request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	processor := NewTikaProcessor(TikaConfig{URL: server.URL, Timeout: time.Second, MaxBytes: 4})
	result, err := processor.Process(context.Background(), Input{
		Object: ObjectRef{Bucket: "docs", Key: "large.txt"},
		OpenContent: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("12345")), nil
		},
	})
	if err != nil {
		t.Fatalf("process oversized unknown-size object: %v", err)
	}
	if result.Status != StatusSkipped {
		t.Fatalf("status = %q, want %q", result.Status, StatusSkipped)
	}
	if result.Metadata["reason"] != "object exceeds tika max bytes" {
		t.Fatalf("reason = %q, want max bytes skip", result.Metadata["reason"])
	}
}
