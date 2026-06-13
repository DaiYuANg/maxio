package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestRedisCacheObjectRoundTripAndMiss(t *testing.T) {
	ctx := context.Background()
	client := newFakeRedisClient()
	cache := NewRedisCache(client, WithRedisPrefix("test-cache"), WithRedisTTL(time.Minute))
	meta := testObjectMeta("photos", "2026/05/a:b.jpg")

	mustSetRedisObject(ctx, t, cache, meta)
	got := requireRedisObjectHit(ctx, t, cache, meta.Bucket, meta.Key)
	requireMemoryMeta(t, got, meta)
	requireRedisObjectMiss(ctx, t, cache, meta.Bucket, "missing")
	requireRedisSetCall(t, client, cache.objectKey(meta.Bucket, meta.Key), time.Minute)
}

func TestRedisCacheListObjectsRoundTrip(t *testing.T) {
	ctx := context.Background()
	client := newFakeRedisClient()
	cache := NewRedisCache(client, WithRedisPrefix("test-cache"))
	items := []model.ObjectMeta{{Bucket: "docs", Key: "a.txt", Size: 1}, {Bucket: "docs", Key: "b.txt", Size: 2}}

	mustSetRedisList(ctx, t, cache, "docs", "a", items)
	requireRedisListHit(ctx, t, cache, "docs", "a", len(items))
}

func TestRedisCacheInvalidateBucketDeletesObjectAndListKeys(t *testing.T) {
	ctx := context.Background()
	client := newFakeRedisClient()
	cache := NewRedisCache(client, WithRedisPrefix("test-cache"), WithRedisScanCount(2))
	objectKey := cache.objectKey("bucket-a", "a.txt")
	listKey := cache.listKey("bucket-a", "")
	otherKey := cache.objectKey("bucket-b", "b.txt")

	seedRedisKeys(client, objectKey, listKey, otherKey)
	client.scanBatches = []scanBatch{{keys: []string{objectKey}, cursor: 1}, {keys: []string{listKey}, cursor: 0}}
	mustInvalidateRedisBucket(ctx, t, cache, "bucket-a")
	requireRedisKeyMissing(t, client, objectKey)
	requireRedisKeyMissing(t, client, listKey)
	requireRedisKeyPresent(t, client, otherKey)
	requireRedisScanCalls(t, client, cache.bucketPattern("bucket-a"), 2, 2)
}

func TestRedisCacheDeleteObjectInvalidatesBucketLists(t *testing.T) {
	ctx := context.Background()
	client := newFakeRedisClient()
	cache := NewRedisCache(client, WithRedisPrefix("test-cache"))
	objectKey := cache.objectKey("docs", "a.txt")
	listKey := cache.listKey("docs", "")

	seedRedisKeys(client, objectKey, listKey)
	client.scanBatches = []scanBatch{{keys: []string{listKey}, cursor: 0}}
	mustDeleteRedisObject(ctx, t, cache, "docs", "a.txt")
	requireRedisKeyMissing(t, client, objectKey)
	requireRedisKeyMissing(t, client, listKey)
}

func TestRedisCacheReturnsRedisErrorsAndCloses(t *testing.T) {
	ctx := context.Background()
	client := newFakeRedisClient()
	client.err = errors.New("redis unavailable")
	cache := NewRedisCache(client)

	requireRedisGetError(ctx, t, cache)
	client.err = nil
	client.closeErr = errors.New("close failed")
	requireRedisCloseError(t, cache, client)
}

func mustSetRedisObject(ctx context.Context, t *testing.T, cache *RedisCache, meta model.ObjectMeta) {
	t.Helper()
	if err := cache.SetObject(ctx, meta); err != nil {
		t.Fatalf("set object: %v", err)
	}
}

func mustSetRedisList(ctx context.Context, t *testing.T, cache *RedisCache, bucket, prefix string, items []model.ObjectMeta) {
	t.Helper()
	if err := cache.SetListObjects(ctx, bucket, prefix, items); err != nil {
		t.Fatalf("set list objects: %v", err)
	}
}

func mustInvalidateRedisBucket(ctx context.Context, t *testing.T, cache *RedisCache, bucket string) {
	t.Helper()
	if err := cache.InvalidateBucket(ctx, bucket); err != nil {
		t.Fatalf("invalidate bucket: %v", err)
	}
}

func mustDeleteRedisObject(ctx context.Context, t *testing.T, cache *RedisCache, bucket, key string) {
	t.Helper()
	if err := cache.DeleteObject(ctx, bucket, key); err != nil {
		t.Fatalf("delete object: %v", err)
	}
}

func requireRedisObjectHit(ctx context.Context, t *testing.T, cache *RedisCache, bucket, key string) model.ObjectMeta {
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

func requireRedisObjectMiss(ctx context.Context, t *testing.T, cache *RedisCache, bucket, key string) {
	t.Helper()
	_, ok, err := cache.GetObject(ctx, bucket, key)
	if err != nil {
		t.Fatalf("get missing object: %v", err)
	}
	if ok {
		t.Fatalf("expected object cache miss for %s/%s", bucket, key)
	}
}

func requireRedisListHit(ctx context.Context, t *testing.T, cache *RedisCache, bucket, prefix string, want int) {
	t.Helper()
	items, ok, err := cache.GetListObjects(ctx, bucket, prefix)
	if err != nil {
		t.Fatalf("get list objects: %v", err)
	}
	if !ok || len(items) != want {
		t.Fatalf("list cache hit = %v len = %d, want hit len %d", ok, len(items), want)
	}
}

func requireRedisSetCall(t *testing.T, client *fakeRedisClient, key string, ttl time.Duration) {
	t.Helper()
	if len(client.calls) < 1 || client.calls[0].name != "set" {
		t.Fatalf("expected first set call, got %#v", client.calls)
	}
	if client.calls[0].key != key || client.calls[0].expiration != ttl {
		t.Fatalf("unexpected set call: %#v", client.calls[0])
	}
}

func requireRedisScanCalls(t *testing.T, client *fakeRedisClient, match string, count int64, want int) {
	t.Helper()
	scanCalls := 0
	for _, call := range client.calls {
		if call.name != "scan" {
			continue
		}
		scanCalls++
		if call.match != match || call.count != count {
			t.Fatalf("unexpected scan call: %#v", call)
		}
	}
	if scanCalls != want {
		t.Fatalf("scan calls = %d, want %d", scanCalls, want)
	}
}

func requireRedisKeyMissing(t *testing.T, client *fakeRedisClient, key string) {
	t.Helper()
	if _, ok := client.values[key]; ok {
		t.Fatalf("expected redis key %q deleted", key)
	}
}

func requireRedisKeyPresent(t *testing.T, client *fakeRedisClient, key string) {
	t.Helper()
	if _, ok := client.values[key]; !ok {
		t.Fatalf("expected redis key %q retained", key)
	}
}

func requireRedisGetError(ctx context.Context, t *testing.T, cache *RedisCache) {
	t.Helper()
	if _, _, err := cache.GetObject(ctx, "bucket", "key"); err == nil {
		t.Fatal("expected redis error")
	}
}

func requireRedisCloseError(t *testing.T, cache *RedisCache, client *fakeRedisClient) {
	t.Helper()
	if err := cache.Close(); err == nil {
		t.Fatal("expected close error")
	}
	if !client.closed {
		t.Fatal("expected client closed")
	}
}

func seedRedisKeys(client *fakeRedisClient, keys ...string) {
	for _, key := range keys {
		client.values[key] = "{}"
	}
}
