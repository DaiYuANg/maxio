package cache

import (
	"context"

	"github.com/lyonbrown4d/maxio/internal/model"
)

// GetObjectVersion returns a cached current object version.
func (c *MemoryMetadataCache) GetObjectVersion(
	_ context.Context,
	bucket string,
	key string,
) (model.ObjectVersion, bool, error) {
	if c == nil || c.closed.Load() {
		return model.ObjectVersion{}, false, nil
	}
	cacheKey := objectVersionCacheKey(bucket, key)
	value, ok := c.cache.Get(cacheKey)
	if !ok {
		return model.ObjectVersion{}, false, nil
	}
	version, ok := value.(model.ObjectVersion)
	if !ok {
		c.cache.Del(cacheKey)
		return model.ObjectVersion{}, false, nil
	}
	return cloneObjectVersion(version), true, nil
}

// SetObjectVersion caches a current object version and invalidates bucket listing entries.
func (c *MemoryMetadataCache) SetObjectVersion(_ context.Context, version model.ObjectVersion) error {
	if c == nil || c.closed.Load() {
		return nil
	}
	c.invalidateBucketLists(version.Bucket)
	cacheKey := objectVersionCacheKey(version.Bucket, version.Key)
	if c.cache.SetWithTTL(cacheKey, cloneObjectVersion(version), estimateObjectVersionCost(version), c.ttl) {
		c.remember(version.Bucket, cacheKey)
	}
	return nil
}

// DeleteObjectVersion removes a cached current object version and invalidates bucket listing entries.
func (c *MemoryMetadataCache) DeleteObjectVersion(_ context.Context, bucket, key string) error {
	if c == nil || c.closed.Load() {
		return nil
	}
	cacheKey := objectVersionCacheKey(bucket, key)
	c.cache.Del(cacheKey)
	c.forget(bucket, cacheKey)
	c.invalidateBucketLists(bucket)
	return nil
}

// GetDigestRef returns a cached digest reference.
func (c *MemoryMetadataCache) GetDigestRef(_ context.Context, digest string) (model.DigestRef, bool, error) {
	if c == nil || c.closed.Load() {
		return model.DigestRef{}, false, nil
	}
	cacheKey := digestRefCacheKey(digest)
	value, ok := c.cache.Get(cacheKey)
	if !ok {
		return model.DigestRef{}, false, nil
	}
	ref, ok := value.(model.DigestRef)
	if !ok {
		c.cache.Del(cacheKey)
		return model.DigestRef{}, false, nil
	}
	return cloneDigestRef(ref), true, nil
}

// SetDigestRef caches a digest reference.
func (c *MemoryMetadataCache) SetDigestRef(_ context.Context, ref model.DigestRef) error {
	if c == nil || c.closed.Load() {
		return nil
	}
	c.cache.SetWithTTL(digestRefCacheKey(ref.Digest), cloneDigestRef(ref), estimateDigestRefCost(ref), c.ttl)
	return nil
}

// DeleteDigestRef removes a cached digest reference.
func (c *MemoryMetadataCache) DeleteDigestRef(_ context.Context, digest string) error {
	if c == nil || c.closed.Load() {
		return nil
	}
	c.cache.Del(digestRefCacheKey(digest))
	return nil
}
