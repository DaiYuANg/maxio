package object_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/index"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
	"github.com/lyonbrown4d/maxio/object"
)

func TestRebuildIndexIndexesObjectContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	objects := newIndexTestService(t)
	if err := objects.CreateBucket(ctx, "docs"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	if _, err := objects.PutObject(ctx, "docs", "guide.txt", strings.NewReader("needle searchable content"), object.PutOptions{
		ContentType: "text/plain",
	}); err != nil {
		t.Fatalf("put object: %v", err)
	}

	result, err := objects.RebuildIndex(ctx)
	if err != nil {
		t.Fatalf("rebuild index: %v", err)
	}
	if result.Objects != 1 || result.Failed != 0 {
		t.Fatalf("rebuild result = %+v", result)
	}

	search, err := objects.Search(ctx, model.SearchQuery{Query: "needle"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(search.Items) != 1 {
		t.Fatalf("search hits = %d, want 1", len(search.Items))
	}
	status := objects.IndexStatus()
	if status.LastRebuildObjects != 1 || status.LastRebuildFailed != 0 {
		t.Fatalf("index status = %+v", status)
	}
}

func TestRebuildIndexRemovesDeletedObject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	objects := newIndexTestService(t)
	createIndexTestBucket(ctx, t, objects)
	putIndexTestObject(ctx, t, objects, "stale.txt", "stale searchable content")
	rebuildIndex(ctx, t, objects, "initial")
	assertSearchHits(ctx, t, objects, "stale", 1)
	if _, err := objects.DeleteObject(ctx, "docs", "stale.txt"); err != nil {
		t.Fatalf("delete object: %v", err)
	}

	result := rebuildIndex(ctx, t, objects, "after delete")
	if result.Objects != 0 || result.Failed != 0 {
		t.Fatalf("rebuild result = %+v", result)
	}
	assertSearchHits(ctx, t, objects, "stale", 0)
}

func newIndexTestService(t *testing.T) *object.Service {
	t.Helper()

	storage, err := store.NewStore(t.TempDir(), metadata.NewInMemoryMetadata(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return object.NewService(storage, index.NewInMemorySearchEngine(), nil, slog.New(slog.DiscardHandler), object.Config{})
}

func createIndexTestBucket(ctx context.Context, t *testing.T, objects *object.Service) {
	t.Helper()

	if err := objects.CreateBucket(ctx, "docs"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
}

func putIndexTestObject(ctx context.Context, t *testing.T, objects *object.Service, key, content string) {
	t.Helper()

	_, err := objects.PutObject(ctx, "docs", key, strings.NewReader(content), object.PutOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
}

func rebuildIndex(ctx context.Context, t *testing.T, objects *object.Service, label string) object.IndexRebuildResult {
	t.Helper()

	result, err := objects.RebuildIndex(ctx)
	if err != nil {
		t.Fatalf("%s rebuild index: %v", label, err)
	}
	return result
}

func assertSearchHits(ctx context.Context, t *testing.T, objects *object.Service, query string, want int) {
	t.Helper()

	search, err := objects.Search(ctx, model.SearchQuery{Query: query})
	if err != nil {
		t.Fatalf("search %q: %v", query, err)
	}
	if len(search.Items) != want {
		t.Fatalf("search hits for %q = %d, want %d", query, len(search.Items), want)
	}
}
