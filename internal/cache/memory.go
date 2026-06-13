// Package cache provides metadata cache implementations for MaxIO.
package cache

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/ristretto"

	"github.com/lyonbrown4d/maxio/internal/model"
)

const (
	objectKeyPrefix = "object:"
	listKeyPrefix   = "list:"
	bucketsKey      = "buckets"
	keySeparator    = "\x00"

	// DefaultMemoryTTL is the default time-to-live for metadata cache entries.
	DefaultMemoryTTL = 5 * time.Minute
	// DefaultMemoryMaxCost is the default in-memory cache capacity budget.
	DefaultMemoryMaxCost int64 = 64 << 20
	// DefaultMemoryNumCounters is the default admission counter count.
	DefaultMemoryNumCounters int64 = 100_000
	// DefaultMemoryBufferItems is Ristretto's default set buffer size for this cache.
	DefaultMemoryBufferItems int64 = 64
)

// MemoryConfig configures the Ristretto-backed metadata cache.
type MemoryConfig struct {
	TTL         time.Duration
	MaxCost     int64
	NumCounters int64
	BufferItems int64
	Metrics     bool
}

// MemoryMetadataCache is an in-memory MetadataCache backed by Ristretto.
type MemoryMetadataCache struct {
	cache *ristretto.Cache
	ttl   time.Duration

	mu         sync.Mutex
	bucketKeys map[string]map[string]struct{}
	closed     atomic.Bool
}

// NewMemoryMetadataCache creates a Ristretto-backed metadata cache.
func NewMemoryMetadataCache(cfg MemoryConfig) (*MemoryMetadataCache, error) {
	cfg = cfg.normalized()

	rc, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: cfg.NumCounters,
		MaxCost:     cfg.MaxCost,
		BufferItems: cfg.BufferItems,
		Metrics:     cfg.Metrics,
	})
	if err != nil {
		return nil, fmt.Errorf("create memory metadata cache: %w", err)
	}

	return &MemoryMetadataCache{
		cache:      rc,
		ttl:        cfg.TTL,
		bucketKeys: make(map[string]map[string]struct{}),
	}, nil
}

// GetBuckets returns cached bucket metadata.
func (c *MemoryMetadataCache) GetBuckets(_ context.Context) ([]model.Bucket, bool, error) {
	if c == nil || c.closed.Load() {
		return nil, false, nil
	}

	value, ok := c.cache.Get(bucketsKey)
	if !ok {
		return nil, false, nil
	}

	buckets, ok := value.([]model.Bucket)
	if !ok {
		c.cache.Del(bucketsKey)
		return nil, false, nil
	}
	return cloneBuckets(buckets), true, nil
}

// SetBuckets caches bucket metadata.
func (c *MemoryMetadataCache) SetBuckets(_ context.Context, buckets []model.Bucket) error {
	if c == nil || c.closed.Load() {
		return nil
	}
	if buckets == nil {
		c.cache.Del(bucketsKey)
		return nil
	}
	c.cache.SetWithTTL(bucketsKey, cloneBuckets(buckets), estimateBucketsCost(buckets), c.ttl)
	return nil
}

// GetObject returns a cached object metadata entry.
func (c *MemoryMetadataCache) GetObject(_ context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	if c == nil || c.closed.Load() {
		return model.ObjectMeta{}, false, nil
	}

	value, ok := c.cache.Get(objectCacheKey(bucket, key))
	if !ok {
		return model.ObjectMeta{}, false, nil
	}

	meta, ok := value.(model.ObjectMeta)
	if !ok {
		c.cache.Del(objectCacheKey(bucket, key))
		return model.ObjectMeta{}, false, nil
	}

	return cloneObjectMeta(meta), true, nil
}

// SetObject caches an object metadata entry and invalidates bucket listing entries.
func (c *MemoryMetadataCache) SetObject(_ context.Context, meta model.ObjectMeta) error {
	if c == nil || c.closed.Load() {
		return nil
	}

	c.invalidateBucketLists(meta.Bucket)
	key := objectCacheKey(meta.Bucket, meta.Key)
	if c.cache.SetWithTTL(key, cloneObjectMeta(meta), estimateObjectCost(meta), c.ttl) {
		c.remember(meta.Bucket, key)
	}
	return nil
}

// DeleteObject removes an object metadata entry and invalidates bucket listing entries.
func (c *MemoryMetadataCache) DeleteObject(_ context.Context, bucket, key string) error {
	if c == nil || c.closed.Load() {
		return nil
	}

	cacheKey := objectCacheKey(bucket, key)
	c.cache.Del(cacheKey)
	c.forget(bucket, cacheKey)
	c.invalidateBucketLists(bucket)
	return nil
}

// GetListObjects returns a cached object listing for a bucket and prefix.
func (c *MemoryMetadataCache) GetListObjects(
	_ context.Context,
	bucket string,
	prefix string,
) ([]model.ObjectMeta, bool, error) {
	if c == nil || c.closed.Load() {
		return nil, false, nil
	}

	key := listCacheKey(bucket, prefix)
	value, ok := c.cache.Get(key)
	if !ok {
		return nil, false, nil
	}

	items, ok := value.([]model.ObjectMeta)
	if !ok {
		c.cache.Del(key)
		return nil, false, nil
	}

	return cloneObjectMetaSlice(items), true, nil
}

// SetListObjects caches an object listing for a bucket and prefix.
func (c *MemoryMetadataCache) SetListObjects(
	_ context.Context,
	bucket string,
	prefix string,
	items []model.ObjectMeta,
) error {
	if c == nil || c.closed.Load() {
		return nil
	}

	key := listCacheKey(bucket, prefix)
	values := cloneObjectMetaSlice(items)
	if c.cache.SetWithTTL(key, values, estimateListCost(bucket, prefix, values), c.ttl) {
		c.remember(bucket, key)
	}
	return nil
}

// InvalidateBucket removes all cached metadata entries for a bucket.
func (c *MemoryMetadataCache) InvalidateBucket(_ context.Context, bucket string) error {
	if c == nil || c.closed.Load() {
		return nil
	}

	for _, key := range c.forgetBucket(bucket) {
		c.cache.Del(key)
	}
	c.cache.Del(bucketsKey)
	return nil
}

// InvalidateAll removes all tracked cached metadata entries.
func (c *MemoryMetadataCache) InvalidateAll(_ context.Context) error {
	if c == nil || c.closed.Load() {
		return nil
	}

	c.cache.Del(bucketsKey)
	c.mu.Lock()
	keysByBucket := c.bucketKeys
	c.bucketKeys = make(map[string]map[string]struct{})
	c.mu.Unlock()
	for _, keys := range keysByBucket {
		for key := range keys {
			c.cache.Del(key)
		}
	}
	return nil
}

// Close releases cache resources.
func (c *MemoryMetadataCache) Close() error {
	if c == nil || !c.closed.CompareAndSwap(false, true) {
		return nil
	}

	c.cache.Close()
	c.mu.Lock()
	c.bucketKeys = nil
	c.mu.Unlock()
	return nil
}

// Wait blocks until pending Ristretto writes have been processed.
func (c *MemoryMetadataCache) Wait() {
	if c == nil || c.closed.Load() {
		return
	}
	c.cache.Wait()
}
