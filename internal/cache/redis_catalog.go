package cache

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lyonbrown4d/maxio/internal/model"
)

// GetObjectVersion returns a cached current object version.
func (cache *RedisCache) GetObjectVersion(
	ctx context.Context,
	bucket string,
	key string,
) (model.ObjectVersion, bool, error) {
	var version model.ObjectVersion
	data, ok, err := cache.getJSON(ctx, cache.objectVersionKey(bucket, key))
	if err != nil || !ok {
		return version, ok, err
	}
	if err := json.Unmarshal(data, &version); err != nil {
		return version, false, fmt.Errorf("decode cached object version: %w", err)
	}
	return cloneObjectVersion(version), true, nil
}

// SetObjectVersion caches a current object version and invalidates bucket list caches.
func (cache *RedisCache) SetObjectVersion(ctx context.Context, version model.ObjectVersion) error {
	if err := cache.setJSON(ctx, cache.objectVersionKey(version.Bucket, version.Key), cloneObjectVersion(version)); err != nil {
		return fmt.Errorf("set cached object version: %w", err)
	}
	if err := cache.invalidatePattern(ctx, cache.listPattern(version.Bucket)); err != nil {
		return fmt.Errorf("invalidate cached object lists: %w", err)
	}
	return nil
}

// DeleteObjectVersion removes a cached current object version and invalidates bucket list caches.
func (cache *RedisCache) DeleteObjectVersion(ctx context.Context, bucket, key string) error {
	if err := cache.kv.Delete(ctx, cache.objectVersionKey(bucket, key)); err != nil {
		return fmt.Errorf("delete cached object version: %w", err)
	}
	if err := cache.invalidatePattern(ctx, cache.listPattern(bucket)); err != nil {
		return fmt.Errorf("invalidate cached object lists: %w", err)
	}
	return nil
}

// GetDigestRef returns a cached digest reference.
func (cache *RedisCache) GetDigestRef(ctx context.Context, digest string) (model.DigestRef, bool, error) {
	var ref model.DigestRef
	data, ok, err := cache.getJSON(ctx, cache.digestRefKey(digest))
	if err != nil || !ok {
		return ref, ok, err
	}
	if err := json.Unmarshal(data, &ref); err != nil {
		return ref, false, fmt.Errorf("decode cached digest ref: %w", err)
	}
	return cloneDigestRef(ref), true, nil
}

// SetDigestRef caches a digest reference.
func (cache *RedisCache) SetDigestRef(ctx context.Context, ref model.DigestRef) error {
	if err := cache.setJSON(ctx, cache.digestRefKey(ref.Digest), cloneDigestRef(ref)); err != nil {
		return fmt.Errorf("set cached digest ref: %w", err)
	}
	return nil
}

// DeleteDigestRef removes a cached digest reference.
func (cache *RedisCache) DeleteDigestRef(ctx context.Context, digest string) error {
	if err := cache.kv.Delete(ctx, cache.digestRefKey(digest)); err != nil {
		return fmt.Errorf("delete cached digest ref: %w", err)
	}
	return nil
}
