package cache

import (
	"context"
	"testing"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestMemoryMetadataCacheObjectGetSetDelete(t *testing.T) {
	ctx := context.Background()
	cache := newTestMemoryCache(t, time.Minute)
	meta := testObjectMeta("photos", "2026/cat.jpg")

	mustSetMemoryObject(ctx, t, cache, meta)
	got := requireMemoryObjectHit(ctx, t, cache, meta.Bucket, meta.Key)
	requireMemoryMeta(t, got, meta)

	got.UserMetadata["owner"] = "mutated"
	gotAgain := requireMemoryObjectHit(ctx, t, cache, meta.Bucket, meta.Key)
	if gotAgain.UserMetadata["owner"] != "alice" {
		t.Fatalf("expected cached metadata isolation, got owner %q", gotAgain.UserMetadata["owner"])
	}

	mustDeleteMemoryObject(ctx, t, cache, meta.Bucket, meta.Key)
	requireMemoryObjectMiss(ctx, t, cache, meta.Bucket, meta.Key)
}

func TestMemoryMetadataCacheDeleteObjectInvalidatesLists(t *testing.T) {
	ctx := context.Background()
	cache := newTestMemoryCache(t, time.Minute)
	meta := testObjectMeta("photos", "2026/cat.jpg")

	mustSetMemoryObject(ctx, t, cache, meta)
	mustSetMemoryList(ctx, t, cache, meta.Bucket, "2026/", list.NewList(meta))
	requireMemoryListHit(ctx, t, cache, meta.Bucket, "2026/", 1)

	mustDeleteMemoryObject(ctx, t, cache, meta.Bucket, meta.Key)
	requireMemoryListMiss(ctx, t, cache, meta.Bucket, "2026/")
}

func TestMemoryMetadataCacheSetObjectInvalidatesLists(t *testing.T) {
	ctx := context.Background()
	cache := newTestMemoryCache(t, time.Minute)
	meta := testObjectMeta("photos", "2026/cat.jpg")

	mustSetMemoryList(ctx, t, cache, meta.Bucket, "2026/", list.NewList(meta))
	requireMemoryListHit(ctx, t, cache, meta.Bucket, "2026/", 1)

	mustSetMemoryObject(ctx, t, cache, meta)
	requireMemoryListMiss(ctx, t, cache, meta.Bucket, "2026/")
}

func TestMemoryMetadataCacheInvalidateBucket(t *testing.T) {
	ctx := context.Background()
	cache := newTestMemoryCache(t, time.Minute)
	photo := testObjectMeta("photos", "2026/cat.jpg")
	video := testObjectMeta("videos", "2026/cat.mp4")

	mustSetMemoryObject(ctx, t, cache, photo)
	mustSetMemoryList(ctx, t, cache, photo.Bucket, "2026/", list.NewList(photo))
	mustSetMemoryObject(ctx, t, cache, video)
	mustInvalidateMemoryBucket(ctx, t, cache, photo.Bucket)

	requireMemoryObjectMiss(ctx, t, cache, photo.Bucket, photo.Key)
	requireMemoryListMiss(ctx, t, cache, photo.Bucket, "2026/")
	requireMemoryObjectHit(ctx, t, cache, video.Bucket, video.Key)
}

func TestMemoryMetadataCacheTTL(t *testing.T) {
	ctx := context.Background()
	cache := newTestMemoryCache(t, 15*time.Millisecond)
	meta := testObjectMeta("photos", "2026/cat.jpg")

	mustSetMemoryObject(ctx, t, cache, meta)
	requireMemoryObjectHit(ctx, t, cache, meta.Bucket, meta.Key)
	time.Sleep(40 * time.Millisecond)
	requireMemoryObjectMiss(ctx, t, cache, meta.Bucket, meta.Key)
}

func newTestMemoryCache(t *testing.T, ttl time.Duration) *MemoryMetadataCache {
	t.Helper()

	cache, err := NewMemoryMetadataCache(MemoryConfig{
		TTL:         ttl,
		MaxCost:     1 << 20,
		NumCounters: 1_000,
		BufferItems: 64,
	})
	if err != nil {
		t.Fatalf("create memory metadata cache: %v", err)
	}
	t.Cleanup(func() {
		if closeErr := cache.Close(); closeErr != nil {
			t.Errorf("close memory metadata cache: %v", closeErr)
		}
	})
	return cache
}

func mustSetMemoryObject(ctx context.Context, t *testing.T, cache *MemoryMetadataCache, meta model.ObjectMeta) {
	t.Helper()
	if err := cache.SetObject(ctx, meta); err != nil {
		t.Fatalf("set object: %v", err)
	}
	cache.Wait()
}

func mustDeleteMemoryObject(ctx context.Context, t *testing.T, cache *MemoryMetadataCache, bucket, key string) {
	t.Helper()
	if err := cache.DeleteObject(ctx, bucket, key); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	cache.Wait()
}

func mustSetMemoryList(
	ctx context.Context,
	t *testing.T,
	cache *MemoryMetadataCache,
	bucket string,
	prefix string,
	items *list.List[model.ObjectMeta],
) {
	t.Helper()
	if err := cache.SetListObjects(ctx, bucket, prefix, items); err != nil {
		t.Fatalf("set list objects: %v", err)
	}
	cache.Wait()
}

func mustInvalidateMemoryBucket(ctx context.Context, t *testing.T, cache *MemoryMetadataCache, bucket string) {
	t.Helper()
	if err := cache.InvalidateBucket(ctx, bucket); err != nil {
		t.Fatalf("invalidate bucket: %v", err)
	}
	cache.Wait()
}

func requireMemoryObjectHit(
	ctx context.Context,
	t *testing.T,
	cache *MemoryMetadataCache,
	bucket string,
	key string,
) model.ObjectMeta {
	t.Helper()
	meta, ok, err := cache.GetObject(ctx, bucket, key)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if !ok {
		t.Fatalf("expected object cache hit for %s/%s", bucket, key)
	}
	return meta
}

func requireMemoryObjectMiss(ctx context.Context, t *testing.T, cache *MemoryMetadataCache, bucket, key string) {
	t.Helper()
	_, ok, err := cache.GetObject(ctx, bucket, key)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	if ok {
		t.Fatalf("expected object cache miss for %s/%s", bucket, key)
	}
}

func requireMemoryListHit(
	ctx context.Context,
	t *testing.T,
	cache *MemoryMetadataCache,
	bucket string,
	prefix string,
	want int,
) {
	t.Helper()
	items, ok, err := cache.GetListObjects(ctx, bucket, prefix)
	if err != nil {
		t.Fatalf("get list objects: %v", err)
	}
	if !ok || items.Len() != want {
		t.Fatalf("list cache hit = %v len = %d, want hit len %d", ok, items.Len(), want)
	}
}

func requireMemoryListMiss(ctx context.Context, t *testing.T, cache *MemoryMetadataCache, bucket, prefix string) {
	t.Helper()
	_, ok, err := cache.GetListObjects(ctx, bucket, prefix)
	if err != nil {
		t.Fatalf("get list objects: %v", err)
	}
	if ok {
		t.Fatalf("expected list cache miss for %s prefix %q", bucket, prefix)
	}
}

func requireMemoryMeta(t *testing.T, got, want model.ObjectMeta) {
	t.Helper()
	if got.Bucket != want.Bucket || got.Key != want.Key || got.ETag != want.ETag {
		t.Fatalf("metadata = %+v, want %+v", got, want)
	}
}

func testObjectMeta(bucket, key string) model.ObjectMeta {
	return model.ObjectMeta{
		Bucket:      bucket,
		Key:         key,
		Hash:        "sha256:cat",
		ETag:        "etag-cat",
		Size:        1234,
		ContentType: "image/jpeg",
		UserMetadata: map[string]string{
			"owner": "alice",
		},
		UpdatedAt: time.Now().UTC(),
		State:     model.ObjectStateCommitted,
	}
}
