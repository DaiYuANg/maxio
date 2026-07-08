package processing

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	collectionset "github.com/arcgolabs/collectionx/set"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
		assertTikaExtractionRequest(t, r)
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`[{"X-TIKA:content":"extracted text","Content-Type":"text/plain","X-Parsed-By":["org.apache.tika.parser.DefaultParser"]}]`)); err != nil {
			t.Fatalf("write response: %v", err)
		}
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
	assertTikaExtractionResult(t, result)
}

func assertTikaExtractionRequest(t *testing.T, r *http.Request) {
	t.Helper()
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
}

func assertTikaExtractionResult(t *testing.T, result ProcessorResult) {
	t.Helper()
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
			return nil, errors.New("unexpected OpenContent call")
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
	listener, err := new(net.ListenConfig).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	go runClamAVTestServer(listener, response, done)
	return listener.Addr().String(), func() {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("close clamav test listener: %v", err)
		}
		<-done
	}
}

func runClamAVTestServer(listener net.Listener, response string, done chan<- struct{}) {
	defer close(done)
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	defer closeResource(conn)
	reader := bufio.NewReader(conn)
	if !readClamAVTestStream(reader) {
		return
	}
	if _, err := conn.Write([]byte(response + "\x00")); err != nil {
		return
	}
}

func readClamAVTestStream(reader *bufio.Reader) bool {
	command, err := reader.ReadString(0)
	if err != nil || command != "zINSTREAM\x00" {
		return false
	}
	for {
		var size uint32
		if err := binary.Read(reader, binary.BigEndian, &size); err != nil {
			return false
		}
		if size == 0 {
			return true
		}
		if _, err := io.CopyN(io.Discard, reader, int64(size)); err != nil {
			return false
		}
	}
}
