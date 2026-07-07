package processing

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/maxio/internal/model"
)

type testProcessor struct {
	name   string
	status string
	err    error
}

func (p testProcessor) Name() string {
	if p.name == "" {
		return "test"
	}
	return p.name
}

func (p testProcessor) Capabilities() *collectionset.Set[Capability] {
	return collectionset.NewSet[Capability](CapabilityPolicyEvaluation)
}

func (p testProcessor) Process(context.Context, Input) (ProcessorResult, error) {
	return ProcessorResult{Processor: p.Name(), Status: p.status}, p.err
}

func TestInlineStrictProcessingBlocksDeniedObject(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		testProcessor{status: StatusBlocked},
	)
	input := Input{Object: ObjectRef{Bucket: "docs", Key: "blocked.txt", VersionID: "v1"}}

	err := service.ProcessBeforeCommit(context.Background(), input)
	if !errors.Is(err, ErrProcessingDenied) {
		t.Fatalf("ProcessBeforeCommit error = %v, want ErrProcessingDenied", err)
	}
	record, found := service.Record(context.Background(), input.Object)
	if !found {
		t.Fatal("expected processing record")
	}
	if record.Status != StatusBlocked {
		t.Fatalf("record status = %q, want %q", record.Status, StatusBlocked)
	}
}

func TestAsyncStrictReadWaitsForProcessingRecord(t *testing.T) {
	service := NewService(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncStrict, Timeout: time.Second},
	)
	object := ObjectRef{Bucket: "docs", Key: "pending.txt", VersionID: "v1"}

	err := service.EnsureReadAllowed(context.Background(), object)
	if !errors.Is(err, ErrProcessingPending) {
		t.Fatalf("EnsureReadAllowed error = %v, want ErrProcessingPending", err)
	}
}

func TestTikaProcessorExtractsText(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rmeta/text" {
			t.Fatalf("path = %q, want /rmeta/text", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Fatalf("method = %q, want PUT", r.Method)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Fatalf("accept = %q, want application/json", r.Header.Get("Accept"))
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		if string(body) != "hello tika" {
			t.Fatalf("body = %q, want hello tika", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"X-TIKA:content":"extracted text","Content-Type":"text/plain","X-Parsed-By":["org.apache.tika.parser.DefaultParser"]}]`))
	}))
	defer server.Close()

	processor := NewTikaProcessor(TikaConfig{URL: server.URL, Timeout: time.Second, MaxBytes: 1024})
	result, err := processor.Process(context.Background(), Input{
		Object: ObjectRef{Bucket: "docs", Key: "doc.txt", VersionID: "v1", Size: int64(len("hello tika")), ContentType: "text/plain"},
		OpenContent: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("hello tika")), nil
		},
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if result.Status != StatusSucceeded {
		t.Fatalf("status = %q, want %q", result.Status, StatusSucceeded)
	}
	if result.Metadata["text_bytes"] != "14" {
		t.Fatalf("text_bytes = %q, want 14", result.Metadata["text_bytes"])
	}
	if result.Metadata["detected_content_type"] != "text/plain" {
		t.Fatalf("detected_content_type = %q, want text/plain", result.Metadata["detected_content_type"])
	}
	if result.Metadata["parsed_by"] != "org.apache.tika.parser.DefaultParser" {
		t.Fatalf("parsed_by = %q, want DefaultParser", result.Metadata["parsed_by"])
	}
}

func TestTikaProcessorSkipsOversizedObject(t *testing.T) {
	processor := NewTikaProcessor(TikaConfig{URL: "http://127.0.0.1:1", Timeout: time.Second, MaxBytes: 4})
	result, err := processor.Process(context.Background(), Input{
		Object: ObjectRef{Bucket: "docs", Key: "large.txt", VersionID: "v1", Size: 8},
		OpenContent: func(context.Context) (io.ReadCloser, error) {
			t.Fatal("OpenContent should not be called for oversized object")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if result.Status != StatusSkipped {
		t.Fatalf("status = %q, want %q", result.Status, StatusSkipped)
	}
}

func TestClamAVProcessorReportsCleanObject(t *testing.T) {
	address, closeServer := startClamAVTestServer(t, "stream: OK")
	defer closeServer()

	processor := NewClamAVProcessor(ClamAVConfig{Address: address, Timeout: time.Second})
	result, err := processor.Process(context.Background(), Input{
		Object: ObjectRef{Bucket: "docs", Key: "clean.txt", VersionID: "v1", Size: 5},
		OpenContent: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("clean")), nil
		},
	})
	if err != nil {
		t.Fatalf("Process error: %v", err)
	}
	if result.Status != StatusSucceeded {
		t.Fatalf("status = %q, want %q", result.Status, StatusSucceeded)
	}
	if result.Metadata["verdict"] != "clean" {
		t.Fatalf("verdict = %q, want clean", result.Metadata["verdict"])
	}
}

func TestClamAVProcessorBlocksInfectedObject(t *testing.T) {
	address, closeServer := startClamAVTestServer(t, "stream: Eicar-Test-Signature FOUND")
	defer closeServer()

	processor := NewClamAVProcessor(ClamAVConfig{Address: address, Timeout: time.Second})
	result, err := processor.Process(context.Background(), Input{
		Object: ObjectRef{Bucket: "docs", Key: "bad.txt", VersionID: "v1", Size: 5},
		OpenContent: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("eicar")), nil
		},
	})
	if !errors.Is(err, ErrProcessingDenied) {
		t.Fatalf("Process error = %v, want ErrProcessingDenied", err)
	}
	if result.Status != StatusBlocked {
		t.Fatalf("status = %q, want %q", result.Status, StatusBlocked)
	}
	if result.Metadata["verdict"] != "infected" {
		t.Fatalf("verdict = %q, want infected", result.Metadata["verdict"])
	}
	if result.Metadata["signature"] != "Eicar-Test-Signature" {
		t.Fatalf("signature = %q, want Eicar-Test-Signature", result.Metadata["signature"])
	}
}

func startClamAVTestServer(t *testing.T, response string) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		command, err := reader.ReadString(0)
		if err != nil || command != "zINSTREAM\x00" {
			return
		}
		for {
			var size uint32
			if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
				return
			}
			if size == 0 {
				break
			}
			if _, err := io.CopyN(io.Discard, reader, int64(size)); err != nil {
				return
			}
		}
		_, _ = conn.Write([]byte(response + "\x00"))
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		<-done
	}
}
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
		_, _ = io.Copy(io.Discard, r.Body)
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
			return ProcessorResult{Processor: p.name, Status: StatusFailed}, ctx.Err()
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
	if _, found := findProcessorResult(record.Results, "noop", ModeInlineStrict); !found {
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

func TestDigestDiscardDoesNotTombstoneFuturePreflight(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeInlineStrict),
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "retry.txt", Digest: "sha256:retry"}
	service.Discard(ctx, object)
	if err := service.ProcessBeforeCommit(ctx, Input{Object: object}); err != nil {
		t.Fatalf("preflight after digest discard: %v", err)
	}
	if _, found := service.Record(ctx, object); !found {
		t.Fatal("expected digest preflight record after retry")
	}
}

func TestDiscardHidesExistingVersionRecord(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeAsyncPermissive),
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "discarded.txt", VersionID: "v1"}
	if err := service.storeRecord(ctx, object, StatusSucceeded, "", collectionlist.NewList[ProcessorResult]()); err != nil {
		t.Fatalf("store processing record: %v", err)
	}
	service.Discard(ctx, object)
	if _, found := service.Record(ctx, object); found {
		t.Fatal("expected discarded version record to be hidden")
	}
}

func TestDiscardTombstonesAreBounded(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeAsyncPermissive),
	)
	ctx := context.Background()
	for i := 0; i < maxDiscardedTombstones+1; i++ {
		service.Discard(ctx, ObjectRef{Bucket: "docs", Key: "bounded.txt", VersionID: fmt.Sprintf("v%d", i)})
	}
	if len(service.discarded) != maxDiscardedTombstones {
		t.Fatalf("discarded len = %d, want %d", len(service.discarded), maxDiscardedTombstones)
	}
	first := ObjectRef{Bucket: "docs", Key: "bounded.txt", VersionID: "v0"}
	if service.isDiscarded(first) {
		t.Fatal("expected oldest tombstone to be evicted")
	}
	last := ObjectRef{Bucket: "docs", Key: "bounded.txt", VersionID: fmt.Sprintf("v%d", maxDiscardedTombstones)}
	if !service.isDiscarded(last) {
		t.Fatal("expected newest tombstone to be retained")
	}
}

func TestDiscardTombstoneDuplicateDoesNotGrowOrder(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeAsyncPermissive),
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "duplicate-discard.txt", VersionID: "v1"}
	service.Discard(ctx, object)
	service.Discard(ctx, object)
	if len(service.discardedOrder) != 1 {
		t.Fatalf("discarded order len = %d, want 1", len(service.discardedOrder))
	}
}

type hookRecordStore struct {
	record   model.ProcessingRecord
	records  *collectionlist.List[model.ProcessingRecord]
	onUpsert func()
	onGet    func()
	onDelete func()
}

func (s hookRecordStore) UpsertProcessingRecord(_ context.Context, record model.ProcessingRecord) (model.ProcessingRecord, error) {
	if s.onUpsert != nil {
		s.onUpsert()
	}
	return record, nil
}

func (s hookRecordStore) GetProcessingRecord(context.Context, string, string, string, string) (model.ProcessingRecord, bool, error) {
	if s.onGet != nil {
		s.onGet()
	}
	return s.record, true, nil
}

func (s hookRecordStore) ListProcessingRecords(_ context.Context, _ string, limit int) (*collectionlist.List[model.ProcessingRecord], error) {
	if s.records == nil {
		return collectionlist.NewList[model.ProcessingRecord](), nil
	}
	if limit <= 0 || s.records.Len() <= limit {
		return s.records, nil
	}
	return collectionlist.NewList(s.records.Values()[:limit]...), nil
}

func (s hookRecordStore) DeleteProcessingRecord(context.Context, string, string, string, string) (bool, error) {
	if s.onDelete != nil {
		s.onDelete()
	}
	return true, nil
}

func TestLookupRecordRechecksDiscardAfterStoreRead(t *testing.T) {
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "discard-race.txt", VersionID: "v1"}
	var service *Service
	service = NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{
			record: model.ProcessingRecord{Bucket: object.Bucket, Key: object.Key, VersionID: object.VersionID, Mode: ModeAsyncPermissive, Status: StatusSucceeded},
			onGet: func() {
				service.Discard(ctx, object)
			},
		},
	)
	record, found, err := service.LookupRecord(ctx, object)
	if err != nil {
		t.Fatalf("lookup record: %v", err)
	}
	if found {
		t.Fatalf("record = %#v, want hidden after discard", record)
	}
}

func TestVersionDiscardDoesNotHoldLockDuringStoreDelete(t *testing.T) {
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "discard-lock.txt", VersionID: "v1"}
	done := make(chan struct{})
	var service *Service
	service = NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{onDelete: func() {
			service.Record(ctx, object)
		}},
		NewNoopProcessor(),
	)
	go func() {
		defer close(done)
		service.Discard(ctx, object)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("discard held service lock while deleting processing record from store")
	}
}

func TestStoreRecordDoesNotHoldLockDuringStoreUpsert(t *testing.T) {
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "upsert-lock.txt", VersionID: "v1"}
	done := make(chan struct{})
	var service *Service
	service = NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{onUpsert: func() {
			service.Record(ctx, object)
		}},
		NewNoopProcessor(),
	)
	go func() {
		defer close(done)
		_ = service.storeRecord(ctx, object, StatusSucceeded, "", collectionlist.NewList[ProcessorResult]())
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("storeRecord held service lock while upserting processing record")
	}
}

func TestListRecordsFiltersDiscardedStoreRecords(t *testing.T) {
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "discarded-list.txt", VersionID: "v1"}
	storeRecord := model.ProcessingRecord{Bucket: object.Bucket, Key: object.Key, VersionID: object.VersionID, Mode: ModeAsyncPermissive, Status: StatusSucceeded}
	service := NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{records: collectionlist.NewList(storeRecord)},
		NewNoopProcessor(),
	)
	service.Discard(ctx, object)
	records, err := service.ListRecords(ctx, StatusSucceeded, 10)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if records.Len() != 0 {
		t.Fatalf("records len = %d, want tombstoned store records filtered", records.Len())
	}
}

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
