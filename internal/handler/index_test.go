package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/indexcontrol"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestIndexStatusRouteReturnsCurrentStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := metadata.NewInMemoryMetadata()
	if _, err := store.UpsertIndexDocument(ctx, model.IndexDocument{
		Bucket:    "bucket-a",
		Key:       "alpha",
		VersionID: "v1",
		State:     model.IndexDocumentStateIndexed,
		IndexedAt: time.Unix(100, 0).UTC(),
	}); err != nil {
		t.Fatalf("seed indexed document: %v", err)
	}

	service := NewService(
		newDependencies(store, index.NewInMemorySearchEngine()),
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/_index/status", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}

	var payload indexcontrol.Status
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.IndexedObjects != 1 {
		t.Fatalf("indexed_objects = %d, want 1", payload.IndexedObjects)
	}
}

func TestIndexStatusRouteReturnsUnavailableWhenIndexManagerMissing(t *testing.T) {
	t.Parallel()

	service := NewService(
		Dependencies{metadata: metadata.NewInMemoryMetadata(), search: index.NewInMemorySearchEngine()},
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_index/status", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["error"] != "index manager is unavailable" {
		t.Fatalf("error = %q, want %q", payload["error"], "index manager is unavailable")
	}
}

func TestIndexRebuildRouteReturnsAcceptedOnSuccess(t *testing.T) {
	ctx := context.Background()
	store := metadata.NewInMemoryMetadata()
	if err := store.CreateBucket(ctx, "bucket-a"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	if err := store.UpsertObjectMeta(ctx, model.ObjectMeta{
		Bucket: "bucket-a",
		Key:    "alpha",
		Hash:   "sha256:abcdef",
	}); err != nil {
		t.Fatalf("upsert object meta: %v", err)
	}
	if _, err := store.UpsertObjectRecord(ctx, model.ObjectRecord{
		Bucket:           "bucket-a",
		Key:              "alpha",
		CurrentVersionID: "v1",
	}); err != nil {
		t.Fatalf("upsert object record: %v", err)
	}

	searchEngine := index.NewInMemorySearchEngine()
	clock := func() func() time.Time {
		start := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
		tick := false
		return func() time.Time {
			if !tick {
				tick = true
				return start
			}
			return start.Add(time.Second)
		}
	}()

	manager := indexcontrol.NewManager(
		store,
		searchEngine,
		indexcontrol.ManagerOptions{Now: clock},
	)
	service := NewService(
		Dependencies{metadata: store, search: searchEngine, indexManager: manager},
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(ctx, http.MethodPost, "/_index/rebuild", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusAccepted)
	}

	var payload indexcontrol.RebuildResult
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Objects != 1 {
		t.Fatalf("objects = %d, want 1", payload.Objects)
	}
	if payload.Failed != 0 {
		t.Fatalf("failed = %d, want 0", payload.Failed)
	}
}

func TestIndexRebuildRouteReturnsUnavailableWhenIndexManagerMissing(t *testing.T) {
	service := NewService(
		Dependencies{metadata: metadata.NewInMemoryMetadata(), search: index.NewInMemorySearchEngine()},
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/_index/rebuild", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["error"] != "index manager is unavailable" {
		t.Fatalf("error = %q, want %q", payload["error"], "index manager is unavailable")
	}
}

func TestIndexRebuildRouteReturnsConflictWhenRebuildAlreadyRunning(t *testing.T) {
	requestContext := context.Background()
	listBucketsSeen := make(chan struct{})
	listBucketsGo := make(chan struct{})
	store := &blockingMetadata{
		InMemoryMetadata:  metadata.NewInMemoryMetadata(),
		listBucketsEnter:  listBucketsSeen,
		listBucketsResume: listBucketsGo,
	}
	searchEngine := index.NewInMemorySearchEngine()
	manager := indexcontrol.NewManager(store, searchEngine, indexcontrol.ManagerOptions{})

	service := NewService(
		Dependencies{metadata: store, search: searchEngine, indexManager: manager},
		slog.New(slog.DiscardHandler),
		config.Config{AdminToken: "secret"},
	)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	first := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		request := httptest.NewRequestWithContext(requestContext, http.MethodPost, "/_index/rebuild", http.NoBody)
		request.Header.Set("Authorization", "Bearer secret")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		first <- response
	}()

	select {
	case <-listBucketsSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial rebuild to enter blocked metadata path")
	}

	request := httptest.NewRequestWithContext(requestContext, http.MethodPost, "/_index/rebuild", http.NoBody)
	request.Header.Set("Authorization", "Bearer secret")
	second := httptest.NewRecorder()
	router.ServeHTTP(second, request)
	close(listBucketsGo)
	firstResponse := <-first

	if second.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", second.Code, http.StatusConflict)
	}
	var payload map[string]string
	if err := json.NewDecoder(second.Body).Decode(&payload); err != nil {
		t.Fatalf("decode conflict response: %v", err)
	}
	if payload["error"] != "index rebuild already running" {
		t.Fatalf("error = %q, want %q", payload["error"], "index rebuild already running")
	}
	if firstResponse.Code != http.StatusAccepted {
		t.Fatalf("initial rebuild status = %d, want %d", firstResponse.Code, http.StatusAccepted)
	}
}

type blockingMetadata struct {
	*metadata.InMemoryMetadata
	listBucketsEnter  chan struct{}
	listBucketsResume chan struct{}
	listBucketsOnce   sync.Once
}

func (store *blockingMetadata) ListBuckets(ctx context.Context) (*collectionlist.List[model.Bucket], error) {
	store.listBucketsOnce.Do(func() {
		close(store.listBucketsEnter)
	})
	select {
	case <-store.listBucketsResume:
	case <-ctx.Done():
		return nil, fmt.Errorf("wait for blocked list buckets: %w", ctx.Err())
	}
	buckets, err := store.InMemoryMetadata.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	return buckets, nil
}
