package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestSearchRouteUsesIndexWithoutObjectService(t *testing.T) {
	t.Parallel()

	searchEngine := index.NewInMemorySearchEngine()
	searchEngine.UpsertDocument(model.ObjectMeta{
		Bucket:      "photos",
		Key:         "cats/tabby.jpg",
		Hash:        "sha256:tabby",
		ETag:        "tabby",
		Size:        12,
		ContentType: "image/jpeg",
		UpdatedAt:   time.Unix(100, 0).UTC(),
		State:       model.ObjectStateCommitted,
	}, "searchable tabby")

	deps := newDependencies(metadata.NewInMemoryMetadata(), searchEngine)
	service := NewService(deps, slog.New(slog.DiscardHandler), config.Config{})
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/_search?q=searchable&bucket=photos", http.NoBody)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"key":"cats/tabby.jpg"`) {
		t.Fatalf("search response missing indexed object: %s", recorder.Body.String())
	}
}
