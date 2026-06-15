package cache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/kvx"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/mo"
)

const (
	defaultCachePrefix    = "maxio:metadata"
	defaultCacheTTL       = 5 * time.Minute
	defaultCacheScanCount = int64(100)
	cacheBucketsKey       = "buckets"
)

type metadataJSONCache struct {
	kv        kvx.KV
	prefix    string
	ttl       time.Duration
	scanCount int64
}

func newMetadataJSONCache(kv kvx.KV, prefix string, ttl time.Duration, scanCount int64) *metadataJSONCache {
	return &metadataJSONCache{
		kv:        kv,
		prefix:    normalizeCachePrefix(prefix),
		ttl:       normalizeCacheTTL(ttl),
		scanCount: normalizeScanCount(scanCount),
	}
}

func (cache *metadataJSONCache) GetBuckets(ctx context.Context) (*collectionlist.List[model.Bucket], bool, error) {
	buckets, ok, err := getCacheJSON[[]model.Bucket](ctx, cache, cache.bucketsKey())
	if err != nil || !ok {
		return nil, ok, err
	}
	return cloneBuckets(collectionlist.NewList(buckets...)), true, nil
}

func (cache *metadataJSONCache) SetBuckets(ctx context.Context, buckets *collectionlist.List[model.Bucket]) error {
	if buckets == nil {
		if err := cache.kv.Delete(ctx, cache.bucketsKey()); err != nil {
			return fmt.Errorf("delete cached buckets: %w", err)
		}
		return nil
	}
	return setCacheJSON(ctx, cache, cache.bucketsKey(), cloneBuckets(buckets).Values())
}

func (cache *metadataJSONCache) GetObject(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	meta, ok, err := getCacheJSON[model.ObjectMeta](ctx, cache, cache.objectKey(bucket, key))
	if err != nil || !ok {
		return model.ObjectMeta{}, ok, err
	}
	return cloneObjectMeta(meta), true, nil
}

func (cache *metadataJSONCache) SetObject(ctx context.Context, meta model.ObjectMeta) error {
	if err := setCacheJSON(ctx, cache, cache.objectKey(meta.Bucket, meta.Key), cloneObjectMeta(meta)); err != nil {
		return fmt.Errorf("set cached object: %w", err)
	}
	if err := cache.invalidatePattern(ctx, cache.listPattern(meta.Bucket)); err != nil {
		return fmt.Errorf("invalidate cached object lists: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) DeleteObject(ctx context.Context, bucket, key string) error {
	if err := cache.kv.Delete(ctx, cache.objectKey(bucket, key)); err != nil {
		return fmt.Errorf("delete cached object: %w", err)
	}
	if err := cache.invalidatePattern(ctx, cache.listPattern(bucket)); err != nil {
		return fmt.Errorf("invalidate cached object lists: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) GetListObjects(
	ctx context.Context,
	bucket string,
	prefix string,
) (*collectionlist.List[model.ObjectMeta], bool, error) {
	items, ok, err := getCacheJSON[[]model.ObjectMeta](ctx, cache, cache.listKey(bucket, prefix))
	if err != nil || !ok {
		return nil, ok, err
	}
	return cloneObjectMetaList(collectionlist.NewList(items...)), true, nil
}

func (cache *metadataJSONCache) SetListObjects(
	ctx context.Context,
	bucket string,
	prefix string,
	items *collectionlist.List[model.ObjectMeta],
) error {
	if items == nil {
		return setCacheJSON(ctx, cache, cache.listKey(bucket, prefix), []model.ObjectMeta(nil))
	}
	return setCacheJSON(ctx, cache, cache.listKey(bucket, prefix), cloneObjectMetaList(items).Values())
}

func (cache *metadataJSONCache) GetObjectVersion(
	ctx context.Context,
	bucket string,
	key string,
) (model.ObjectVersion, bool, error) {
	version, ok, err := getCacheJSON[model.ObjectVersion](ctx, cache, cache.objectVersionKey(bucket, key))
	if err != nil || !ok {
		return model.ObjectVersion{}, ok, err
	}
	return cloneObjectVersion(version), true, nil
}

func (cache *metadataJSONCache) SetObjectVersion(ctx context.Context, version model.ObjectVersion) error {
	if err := setCacheJSON(ctx, cache, cache.objectVersionKey(version.Bucket, version.Key), cloneObjectVersion(version)); err != nil {
		return fmt.Errorf("set cached object version: %w", err)
	}
	if err := cache.invalidatePattern(ctx, cache.listPattern(version.Bucket)); err != nil {
		return fmt.Errorf("invalidate cached object lists: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) DeleteObjectVersion(ctx context.Context, bucket, key string) error {
	if err := cache.kv.Delete(ctx, cache.objectVersionKey(bucket, key)); err != nil {
		return fmt.Errorf("delete cached object version: %w", err)
	}
	if err := cache.invalidatePattern(ctx, cache.listPattern(bucket)); err != nil {
		return fmt.Errorf("invalidate cached object lists: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) GetDigestRef(ctx context.Context, digest string) (model.DigestRef, bool, error) {
	ref, ok, err := getCacheJSON[model.DigestRef](ctx, cache, cache.digestRefKey(digest))
	if err != nil || !ok {
		return model.DigestRef{}, ok, err
	}
	return cloneDigestRef(ref), true, nil
}

func (cache *metadataJSONCache) SetDigestRef(ctx context.Context, ref model.DigestRef) error {
	if err := setCacheJSON(ctx, cache, cache.digestRefKey(ref.Digest), cloneDigestRef(ref)); err != nil {
		return fmt.Errorf("set cached digest ref: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) DeleteDigestRef(ctx context.Context, digest string) error {
	if err := cache.kv.Delete(ctx, cache.digestRefKey(digest)); err != nil {
		return fmt.Errorf("delete cached digest ref: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) InvalidateBucket(ctx context.Context, bucket string) error {
	if err := cache.invalidatePattern(ctx, cache.bucketPattern(bucket)); err != nil {
		return fmt.Errorf("invalidate cached bucket: %w", err)
	}
	if err := cache.kv.Delete(ctx, cache.bucketsKey()); err != nil {
		return fmt.Errorf("delete cached buckets: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) InvalidateAll(ctx context.Context) error {
	if err := cache.invalidatePattern(ctx, cache.prefix+":*"); err != nil {
		return fmt.Errorf("invalidate cached metadata: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) invalidatePattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, next, err := cache.kv.Scan(ctx, pattern, cursor, cache.scanCount)
		if err != nil {
			return fmt.Errorf("scan metadata cache: %w", err)
		}
		var cacheKeys []string
		if keys != nil {
			keys.ViewValues(func(values []string) {
				cacheKeys = values
			})
		}
		if len(cacheKeys) > 0 {
			if err := cache.kv.DeleteMulti(ctx, cacheKeys); err != nil {
				return fmt.Errorf("delete metadata cache keys: %w", err)
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

func getCacheJSON[T any](ctx context.Context, cache *metadataJSONCache, key string) (T, bool, error) {
	var value T
	data, err := cache.kv.Get(ctx, key)
	if kvx.IsNil(err) {
		return value, false, nil
	}
	if err != nil {
		return value, false, fmt.Errorf("get metadata cache: %w", err)
	}
	unmarshal := mo.Try(func() (struct{}, error) {
		return struct{}{}, json.Unmarshal(data, &value)
	})
	if unmarshal.IsError() {
		return value, false, fmt.Errorf("decode cached metadata: %w", unmarshal.Error())
	}
	return value, true, nil
}

func setCacheJSON(ctx context.Context, cache *metadataJSONCache, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode metadata cache: %w", err)
	}
	if err := cache.kv.Set(ctx, key, data, cache.ttl); err != nil {
		return fmt.Errorf("set metadata cache: %w", err)
	}
	return nil
}

func (cache *metadataJSONCache) objectKey(bucket, key string) string {
	return cache.prefix + ":object:" + cacheKeyPart(bucket) + ":" + cacheKeyPart(key)
}

func (cache *metadataJSONCache) objectVersionKey(bucket, key string) string {
	return cache.prefix + ":object-version:" + cacheKeyPart(bucket) + ":" + cacheKeyPart(key)
}

func (cache *metadataJSONCache) digestRefKey(digest string) string {
	return cache.prefix + ":digest-ref:" + cacheKeyPart(digest)
}

func (cache *metadataJSONCache) bucketsKey() string {
	return cache.prefix + ":" + cacheBucketsKey
}

func (cache *metadataJSONCache) listKey(bucket, prefix string) string {
	return cache.prefix + ":list:" + cacheKeyPart(bucket) + ":" + cacheKeyPart(prefix)
}

func (cache *metadataJSONCache) bucketPattern(bucket string) string {
	return cache.prefix + ":*:" + cacheKeyPart(bucket) + ":*"
}

func (cache *metadataJSONCache) listPattern(bucket string) string {
	return cache.prefix + ":list:" + cacheKeyPart(bucket) + ":*"
}

func cacheKeyPart(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func normalizeCachePrefix(prefix string) string {
	normalized := strings.Trim(strings.TrimSpace(prefix), ":")
	if normalized == "" {
		return defaultCachePrefix
	}
	return normalized
}

func normalizeCacheTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultCacheTTL
	}
	return ttl
}

func normalizeScanCount(count int64) int64 {
	if count <= 0 {
		return defaultCacheScanCount
	}
	return count
}
