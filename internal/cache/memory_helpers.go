package cache

import (
	"strings"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func (cfg MemoryConfig) normalized() MemoryConfig {
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultMemoryTTL
	}
	if cfg.MaxCost <= 0 {
		cfg.MaxCost = DefaultMemoryMaxCost
	}
	if cfg.NumCounters <= 0 {
		cfg.NumCounters = DefaultMemoryNumCounters
	}
	if cfg.BufferItems <= 0 {
		cfg.BufferItems = DefaultMemoryBufferItems
	}
	return cfg
}

func (c *MemoryMetadataCache) remember(bucket, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return
	}
	if c.bucketKeys[bucket] == nil {
		c.bucketKeys[bucket] = make(map[string]struct{})
	}
	c.bucketKeys[bucket][key] = struct{}{}
}

func (c *MemoryMetadataCache) forget(bucket, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := c.bucketKeys[bucket]
	if len(keys) == 0 {
		return
	}
	delete(keys, key)
	if len(keys) == 0 {
		delete(c.bucketKeys, bucket)
	}
}

func (c *MemoryMetadataCache) forgetBucket(bucket string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := c.bucketKeys[bucket]
	if len(keys) == 0 {
		delete(c.bucketKeys, bucket)
		return nil
	}

	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	delete(c.bucketKeys, bucket)
	return result
}

func (c *MemoryMetadataCache) invalidateBucketLists(bucket string) {
	keys := c.forgetBucketMatching(bucket, func(key string) bool {
		return strings.HasPrefix(key, listKeyPrefix)
	})
	for _, key := range keys {
		c.cache.Del(key)
	}
}

func (c *MemoryMetadataCache) forgetBucketMatching(bucket string, match func(string) bool) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := c.bucketKeys[bucket]
	if len(keys) == 0 {
		return nil
	}

	result := make([]string, 0, len(keys))
	for key := range keys {
		if match(key) {
			result = append(result, key)
			delete(keys, key)
		}
	}
	if len(keys) == 0 {
		delete(c.bucketKeys, bucket)
	}
	return result
}

func objectCacheKey(bucket, key string) string {
	return objectKeyPrefix + bucket + keySeparator + key
}

func objectVersionCacheKey(bucket, key string) string {
	return objectVersionKeyPrefix + bucket + keySeparator + key
}

func digestRefCacheKey(digest string) string {
	return digestRefKeyPrefix + digest
}

func listCacheKey(bucket, prefix string) string {
	return listKeyPrefix + bucket + keySeparator + prefix
}

func estimateObjectCost(meta model.ObjectMeta) int64 {
	cost := int64(256)
	cost += int64(len(meta.Bucket) + len(meta.Key) + len(meta.Hash) + len(meta.ETag))
	cost += int64(len(meta.ContentType) + len(meta.CacheControl) + len(meta.ContentDisposition))
	cost += int64(len(meta.ContentEncoding) + len(meta.ContentLanguage))
	for key, value := range meta.UserMetadata {
		cost += int64(len(key) + len(value) + 32)
	}
	cost += int64(len(meta.ShardPlacements) * 128)
	for _, checksum := range meta.ShardChecksums {
		cost += int64(len(checksum) + 32)
	}
	cost += int64(len(meta.ShardSizes) * 8)
	if meta.WriteIntent != nil {
		cost += int64(128 + len(meta.WriteIntent.ID) + len(meta.WriteIntent.Stage))
	}
	if cost < 1 {
		return 1
	}
	return cost
}

func estimateListCost(bucket, prefix string, items []model.ObjectMeta) int64 {
	cost := int64(128 + len(bucket) + len(prefix))
	for index := range items {
		cost += estimateObjectCost(items[index])
	}
	if cost < 1 {
		return 1
	}
	return cost
}

func estimateObjectVersionCost(version model.ObjectVersion) int64 {
	cost := int64(256)
	cost += int64(len(version.Bucket) + len(version.Key) + len(version.VersionID))
	cost += int64(len(version.Digest) + len(version.ETag) + len(version.ContentType))
	cost += int64(len(version.UpstreamID) + len(version.UpstreamBucket) + len(version.UpstreamKey))
	for key, value := range version.UserMetadata {
		cost += int64(len(key) + len(value) + 32)
	}
	if cost < 1 {
		return 1
	}
	return cost
}

func estimateDigestRefCost(ref model.DigestRef) int64 {
	cost := int64(128)
	cost += int64(len(ref.Digest) + len(ref.UpstreamID) + len(ref.UpstreamBucket) + len(ref.UpstreamKey))
	if cost < 1 {
		return 1
	}
	return cost
}

func estimateBucketsCost(buckets []model.Bucket) int64 {
	cost := int64(128)
	for _, bucket := range buckets {
		cost += int64(64 + len(bucket.Name))
	}
	if cost < 1 {
		return 1
	}
	return cost
}

func cloneObjectMetaSlice(items []model.ObjectMeta) []model.ObjectMeta {
	if items == nil {
		return nil
	}

	clone := make([]model.ObjectMeta, len(items))
	for index := range items {
		clone[index] = cloneObjectMeta(items[index])
	}
	return clone
}
