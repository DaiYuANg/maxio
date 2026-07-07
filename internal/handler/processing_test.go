package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/lyonbrown4d/maxio/internal/processing"
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/status", nil)
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/status", nil)
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/status", nil)
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/records?bucket=docs&key=record.txt&version_id=v1", nil)
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/records?bucket=docs&key=blocked.txt&digest=sha256%3Aeicar", nil)
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
	if payload.Status != processing.StatusBlocked {
		t.Fatalf("status = %q, want %q", payload.Status, processing.StatusBlocked)
	}
	if payload.ReadAllowed {
		t.Fatal("read_allowed = true, want false")
	}
	if payload.ReadBlockReason != "denied" {
		t.Fatalf("read_block_reason = %q, want denied", payload.ReadBlockReason)
	}
	if payload.Digest != "sha256:eicar" {
		t.Fatalf("digest = %q, want sha256:eicar", payload.Digest)
	}
	if len(payload.Results) != 1 || payload.Results[0].Processor != "clamav" {
		t.Fatalf("results = %#v, want clamav result", payload.Results)
	}
	if payload.Results[0].Metadata["verdict"] != "infected" {
		t.Fatalf("verdict = %q, want infected", payload.Results[0].Metadata["verdict"])
	}
}
func TestProcessingRecordRouteRequiresObjectIdentity(t *testing.T) {
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, processing.NewService(slog.New(slog.DiscardHandler), processing.Config{})),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequest(http.MethodGet, "/_processing/records?bucket=docs", nil)
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

func TestProcessingRecordRouteReturnsServerErrorOnLookupFailure(t *testing.T) {
	processor := processing.NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		processing.Config{Enabled: true, Mode: processing.ModeInlineStrict, Timeout: time.Second},
		handlerFailingRecordStore{err: errors.New("metadata offline")},
	)
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, processor),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequest(http.MethodGet, "/_processing/records?bucket=docs&key=record.txt&version_id=v1", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}

func TestProcessingRecordRouteKeepsObjectKeyWhitespace(t *testing.T) {
	processor := processing.NewService(
		slog.New(slog.DiscardHandler),
		processing.Config{Enabled: true, Mode: processing.ModeInlineStrict, Timeout: time.Second},
		processing.NewNoopProcessor(),
	)
	object := processing.ObjectRef{Bucket: "docs", Key: " record.txt", VersionID: "v1"}
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/records?bucket=docs&key=%20record.txt&version_id=v1", nil)
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
	if payload.Key != " record.txt" {
		t.Fatalf("key = %q, want leading-space key", payload.Key)
	}
}

func TestProcessingRecordListRouteByStatus(t *testing.T) {
	processor := processing.NewService(
		slog.New(slog.DiscardHandler),
		processing.Config{Enabled: true, Mode: processing.ModeInlineStrict, Timeout: time.Second},
		processing.NewNoopProcessor(),
	)
	object := processing.ObjectRef{Bucket: "docs", Key: "record-list.txt", VersionID: "v1"}
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/records?status=succeeded&limit=10", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload processingRecordsResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Records) != 1 {
		t.Fatalf("records len = %d, want 1", len(payload.Records))
	}
	if payload.Records[0].Key != "record-list.txt" {
		t.Fatalf("record key = %q, want record-list.txt", payload.Records[0].Key)
	}
}
func TestProcessingRecordListRouteRejectsIdentityParams(t *testing.T) {
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/records?status=succeeded&bucket=docs", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
func TestProcessingRecordListRouteRejectsInvalidStatus(t *testing.T) {
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

	request := httptest.NewRequest(http.MethodGet, "/_processing/records?status=unknown", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}
func TestProcessingStatusRouteWithoutServiceReturnsEmptyCapabilities(t *testing.T) {
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, nil),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequest(http.MethodGet, "/_processing/status", nil)
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
	if payload.Capabilities == nil {
		t.Fatal("expected empty capabilities array, got nil")
	}
	if len(payload.Capabilities) != 0 {
		t.Fatalf("capabilities = %#v, want empty", payload.Capabilities)
	}
	if payload.Processors == nil {
		t.Fatal("expected empty processors array, got nil")
	}
	if len(payload.Processors) != 0 {
		t.Fatalf("processors = %#v, want empty", payload.Processors)
	}
	if payload.ProcessorModes == nil {
		t.Fatal("expected empty processor_modes object, got nil")
	}
	if len(payload.ProcessorModes) != 0 {
		t.Fatalf("processor_modes = %#v, want empty", payload.ProcessorModes)
	}
	if payload.ProcessorFailOpen == nil {
		t.Fatal("expected empty processor_fail_open object, got nil")
	}
	if len(payload.ProcessorFailOpen) != 0 {
		t.Fatalf("processor_fail_open = %#v, want empty", payload.ProcessorFailOpen)
	}
	if payload.Mode != processing.ModeDisabled {
		t.Fatalf("mode = %q, want %q", payload.Mode, processing.ModeDisabled)
	}
	if payload.Timeout != "0s" {
		t.Fatalf("timeout = %q, want 0s", payload.Timeout)
	}
}

func TestProcessingRecordResponseSortsProcessorResults(t *testing.T) {
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, nil),
		slog.New(slog.DiscardHandler),
		config.Config{},
	)
	response := service.processingRecordToResponse(processing.Record{
		Object: processing.ObjectRef{Bucket: "docs", Key: "sorted.txt", VersionID: "v1"},
		Status: processing.StatusSucceeded,
		Results: collectionlist.NewList(
			processing.ProcessorResult{Processor: "tika", Mode: processing.ModeAsyncPermissive, Status: processing.StatusSucceeded},
			processing.ProcessorResult{Processor: "clamav", Mode: processing.ModeInlineStrict, Status: processing.StatusSucceeded},
		),
	})
	if len(response.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(response.Results))
	}
	if response.Results[0].Processor != "clamav" || response.Results[1].Processor != "tika" {
		t.Fatalf("results = %#v, want sorted by processor", response.Results)
	}
}
