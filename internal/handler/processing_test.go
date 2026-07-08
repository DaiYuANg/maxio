package handler

import (
	"context"
	"encoding/json"
	"errors"
	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/lyonbrown4d/maxio/internal/processing"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProcessingStatusRoute(t *testing.T) {
	processor := processing.NewService(
		slog.New(slog.DiscardHandler),
		processing.Config{Enabled: true, Mode: processing.ModeInlineStrict, Timeout: time.Second},
		processing.NewNoopProcessor(),
	)
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, processor),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/status", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload processingStatusResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Enabled {
		t.Fatal("expected enabled processing status")
	}
	if payload.Mode != processing.ModeInlineStrict {
		t.Fatalf("mode = %q, want %q", payload.Mode, processing.ModeInlineStrict)
	}
	if len(payload.Processors) != 1 || payload.Processors[0] != "noop" {
		t.Fatalf("processors = %#v, want [noop]", payload.Processors)
	}
	if payload.ProcessorModes["noop"] != processing.ModeInlineStrict {
		t.Fatalf("noop mode = %q, want %q", payload.ProcessorModes["noop"], processing.ModeInlineStrict)
	}
	if len(payload.ProcessorFailOpen) != 0 {
		t.Fatalf("processor_fail_open = %#v, want empty", payload.ProcessorFailOpen)
	}
}

func TestProcessingStatusRouteReportsProcessorFailOpen(t *testing.T) {
	processor := processing.NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		processing.Config{Enabled: true, Mode: processing.ModeAsyncPermissive, Timeout: time.Second},
		nil,
		processing.BindProcessor(processing.NewTikaProcessor(processing.TikaConfig{FailOpen: true}), processing.ModeAsyncPermissive),
	)
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, processor),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/status", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload processingStatusResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.ProcessorFailOpen["tika"] {
		t.Fatalf("tika fail-open = %v, want true", payload.ProcessorFailOpen["tika"])
	}
}

func TestProcessingStatusRouteRequiresAdminAuth(t *testing.T) {
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, nil),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/status", http.NoBody)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestProcessingRecordRoute(t *testing.T) {
	processor := processing.NewService(
		slog.New(slog.DiscardHandler),
		processing.Config{Enabled: true, Mode: processing.ModeInlineStrict, Timeout: time.Second},
		processing.NewNoopProcessor(),
	)
	object := processing.ObjectRef{Bucket: "docs", Key: "record.txt", VersionID: "v1"}
	if err := processor.ProcessBeforeCommit(context.Background(), processing.Input{Object: object}); err != nil {
		t.Fatalf("seed processing record: %v", err)
	}
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, processor),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/records?bucket=docs&key=record.txt&version_id=v1", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload processingRecordResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Status != processing.StatusSucceeded {
		t.Fatalf("status = %q, want %q", payload.Status, processing.StatusSucceeded)
	}
	if !payload.ReadAllowed {
		t.Fatalf("read_allowed = false, want true")
	}
	if payload.ReadBlockReason != "" {
		t.Fatalf("read_block_reason = %q, want empty", payload.ReadBlockReason)
	}
	if len(payload.Results) != 1 || payload.Results[0].Processor != "noop" {
		t.Fatalf("results = %#v, want noop result", payload.Results)
	}
}

type handlerBlockingProcessor struct{}

func (handlerBlockingProcessor) Name() string {
	return "clamav"
}

func (handlerBlockingProcessor) Capabilities() *collectionset.Set[processing.Capability] {
	return collectionset.NewSet(processing.CapabilityAntivirus)
}

func (handlerBlockingProcessor) Process(context.Context, processing.Input) (processing.ProcessorResult, error) {
	return processing.ProcessorResult{
		Processor: "clamav",
		Status:    processing.StatusBlocked,
		Metadata: map[string]string{
			"verdict":   "infected",
			"signature": "Eicar-Test-Signature",
		},
	}, processing.ErrProcessingDenied
}

func TestProcessingRecordRouteFindsDigestOnlyBlockedRecord(t *testing.T) {
	processor := processing.NewService(
		slog.New(slog.DiscardHandler),
		processing.Config{Enabled: true, Mode: processing.ModeInlineStrict, Timeout: time.Second},
		handlerBlockingProcessor{},
	)
	object := processing.ObjectRef{Bucket: "docs", Key: "blocked.txt", Digest: "sha256:eicar"}
	if err := processor.ProcessBeforeCommit(context.Background(), processing.Input{Object: object}); !errors.Is(err, processing.ErrProcessingDenied) {
		t.Fatalf("seed blocked processing record error = %v, want ErrProcessingDenied", err)
	}
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, processor),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/records?bucket=docs&key=blocked.txt&digest=sha256%3Aeicar", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload processingRecordResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertBlockedProcessingRecordResponse(t, payload)
}

func TestProcessingRecordRouteRequiresObjectIdentity(t *testing.T) {
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, processing.NewService(slog.New(slog.DiscardHandler), processing.Config{})),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/records?bucket=docs", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

type handlerFailingRecordStore struct {
	err error
}

func (s handlerFailingRecordStore) UpsertProcessingRecord(context.Context, model.ProcessingRecord) (model.ProcessingRecord, error) {
	return model.ProcessingRecord{}, s.err
}

func (s handlerFailingRecordStore) GetProcessingRecord(context.Context, string, string, string, string) (model.ProcessingRecord, bool, error) {
	return model.ProcessingRecord{}, false, s.err
}

func (s handlerFailingRecordStore) ListProcessingRecords(context.Context, string, int) (*collectionlist.List[model.ProcessingRecord], error) {
	return nil, s.err
}

func (s handlerFailingRecordStore) DeleteProcessingRecord(context.Context, string, string, string, string) (bool, error) {
	return false, s.err
}
