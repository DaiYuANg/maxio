package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/processing"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/records?bucket=docs&key=record.txt&version_id=v1", http.NoBody)
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

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/records?bucket=docs&key=%20record.txt&version_id=v1", http.NoBody)
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

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/records?status=succeeded&limit=10", http.NoBody)
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

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/records?status=succeeded&bucket=docs", http.NoBody)
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

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_processing/records?status=unknown", http.NoBody)
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
	assertEmptyProcessingStatusResponse(t, payload)
}

func assertBlockedProcessingRecordResponse(t *testing.T, payload processingRecordResponse) {
	t.Helper()
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
	assertBlockedProcessingResult(t, payload.Results)
}

func assertBlockedProcessingResult(t *testing.T, results []processing.ProcessorResult) {
	t.Helper()
	if len(results) != 1 || results[0].Processor != "clamav" {
		t.Fatalf("results = %#v, want clamav result", results)
	}
	if results[0].Metadata["verdict"] != "infected" {
		t.Fatalf("verdict = %q, want infected", results[0].Metadata["verdict"])
	}
}

func assertEmptyProcessingStatusResponse(t *testing.T, payload processingStatusResponse) {
	t.Helper()
	assertEmptyProcessingStatusCollections(t, payload)
	if payload.Mode != processing.ModeDisabled {
		t.Fatalf("mode = %q, want %q", payload.Mode, processing.ModeDisabled)
	}
	if payload.Timeout != "0s" {
		t.Fatalf("timeout = %q, want 0s", payload.Timeout)
	}
}

func assertEmptyProcessingStatusCollections(t *testing.T, payload processingStatusResponse) {
	t.Helper()
	if payload.Capabilities == nil || len(payload.Capabilities) != 0 {
		t.Fatalf("capabilities = %#v, want empty array", payload.Capabilities)
	}
	if payload.Processors == nil || len(payload.Processors) != 0 {
		t.Fatalf("processors = %#v, want empty array", payload.Processors)
	}
	if payload.ProcessorModes == nil || len(payload.ProcessorModes) != 0 {
		t.Fatalf("processor_modes = %#v, want empty object", payload.ProcessorModes)
	}
	if payload.ProcessorFailOpen == nil || len(payload.ProcessorFailOpen) != 0 {
		t.Fatalf("processor_fail_open = %#v, want empty object", payload.ProcessorFailOpen)
	}
}
