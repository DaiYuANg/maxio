// Package cache contains metadata cache implementations.
package cache

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/redis/go-redis/v9"
)

const (
	defaultRedisPrefix    = "maxio:metadata"
	defaultRedisTTL       = 5 * time.Minute
	defaultRedisScanCount = int64(100)
	redisBucketsKey       = "buckets"
)

// RedisClient is the subset of go-redis used by RedisCache.
type RedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd
	Close() error
}

// RedisCache stores metadata cache entries in Redis.
type RedisCache struct {
	client    RedisClient
	prefix    string
	ttl       time.Duration
	scanCount int64
}

// RedisOption configures RedisCache.
type RedisOption func(*RedisCache)

// WithRedisPrefix sets the Redis key prefix.
func WithRedisPrefix(prefix string) RedisOption {
	return func(cache *RedisCache) {
		cache.prefix = normalizeRedisPrefix(prefix)
	}
}

// WithRedisTTL sets the cache TTL.
func WithRedisTTL(ttl time.Duration) RedisOption {
	return func(cache *RedisCache) {
		cache.ttl = ttl
	}
}

// WithRedisScanCount sets the SCAN batch size used by invalidation.
func WithRedisScanCount(count int64) RedisOption {
	return func(cache *RedisCache) {
		if count > 0 {
			cache.scanCount = count
		}
	}
}

// NewRedisCache creates a Redis-backed metadata cache.
func NewRedisCache(client RedisClient, opts ...RedisOption) *RedisCache {
	cache := &RedisCache{
		client:    client,
		prefix:    defaultRedisPrefix,
		ttl:       defaultRedisTTL,
		scanCount: defaultRedisScanCount,
	}
	for _, opt := range opts {
		opt(cache)
	}
	if cache.prefix == "" {
		cache.prefix = defaultRedisPrefix
	}
	if cache.ttl == 0 {
		cache.ttl = defaultRedisTTL
	}
	if cache.scanCount <= 0 {
		cache.scanCount = defaultRedisScanCount
	}
	return cache
}

// GetBuckets returns cached bucket metadata.
func (cache *RedisCache) GetBuckets(ctx context.Context) ([]model.Bucket, bool, error) {
	var buckets []model.Bucket
	data, ok, err := cache.getJSON(ctx, cache.bucketsKey())
	if err != nil || !ok {
		return nil, ok, err
	}
	if err := json.Unmarshal(data, &buckets); err != nil {
		return nil, false, fmt.Errorf("decode cached buckets: %w", err)
	}
	return cloneBuckets(buckets), true, nil
}

// SetBuckets caches bucket metadata.
func (cache *RedisCache) SetBuckets(ctx context.Context, buckets []model.Bucket) error {
	if buckets == nil {
		if err := cache.client.Del(ctx, cache.bucketsKey()).Err(); err != nil {
			return fmt.Errorf("delete cached buckets: %w", err)
		}
		return nil
	}
	return cache.setJSON(ctx, cache.bucketsKey(), cloneBuckets(buckets))
}

// GetObject returns cached object metadata. The second return value is false on cache miss.
func (cache *RedisCache) GetObject(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	var meta model.ObjectMeta
	data, ok, err := cache.getJSON(ctx, cache.objectKey(bucket, key))
	if err != nil || !ok {
		return meta, ok, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, false, fmt.Errorf("decode cached object: %w", err)
	}
	return meta, true, nil
}

// SetObject caches object metadata.
func (cache *RedisCache) SetObject(ctx context.Context, meta model.ObjectMeta) error {
	if err := cache.setJSON(ctx, cache.objectKey(meta.Bucket, meta.Key), meta); err != nil {
		return fmt.Errorf("set cached object: %w", err)
	}
	if err := cache.invalidatePattern(ctx, cache.listPattern(meta.Bucket)); err != nil {
		return fmt.Errorf("invalidate cached object lists: %w", err)
	}
	return nil
}

// DeleteObject removes an object metadata cache entry and invalidates bucket list caches.
func (cache *RedisCache) DeleteObject(ctx context.Context, bucket, key string) error {
	if err := cache.client.Del(ctx, cache.objectKey(bucket, key)).Err(); err != nil {
		return fmt.Errorf("delete cached object: %w", err)
	}
	if err := cache.invalidatePattern(ctx, cache.listPattern(bucket)); err != nil {
		return fmt.Errorf("invalidate cached object lists: %w", err)
	}
	return nil
}

// GetListObjects returns cached ListObjects results. The second return value is false on cache miss.
func (cache *RedisCache) GetListObjects(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, bool, error) {
	var items []model.ObjectMeta
	data, ok, err := cache.getJSON(ctx, cache.listKey(bucket, prefix))
	if err != nil || !ok {
		return nil, ok, err
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, false, fmt.Errorf("decode cached object list: %w", err)
	}
	return items, true, nil
}

// SetListObjects caches ListObjects results for a bucket and prefix.
func (cache *RedisCache) SetListObjects(ctx context.Context, bucket, prefix string, items []model.ObjectMeta) error {
	return cache.setJSON(ctx, cache.listKey(bucket, prefix), items)
}

// InvalidateBucket removes all cached object metadata and ListObjects entries for a bucket.
func (cache *RedisCache) InvalidateBucket(ctx context.Context, bucket string) error {
	if err := cache.invalidatePattern(ctx, cache.bucketPattern(bucket)); err != nil {
		return fmt.Errorf("invalidate cached bucket: %w", err)
	}
	if err := cache.client.Del(ctx, cache.bucketsKey()).Err(); err != nil {
		return fmt.Errorf("delete cached buckets: %w", err)
	}
	return nil
}

// InvalidateAll removes all cached metadata entries with the configured prefix.
func (cache *RedisCache) InvalidateAll(ctx context.Context) error {
	if err := cache.invalidatePattern(ctx, cache.prefix+":*"); err != nil {
		return fmt.Errorf("invalidate cached metadata: %w", err)
	}
	return nil
}

// Close closes the underlying Redis client.
func (cache *RedisCache) Close() error {
	if cache == nil || cache.client == nil {
		return nil
	}
	if err := cache.client.Close(); err != nil {
		return fmt.Errorf("close redis metadata cache: %w", err)
	}
	return nil
}

func (cache *RedisCache) getJSON(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := cache.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get redis metadata cache: %w", err)
	}
	return []byte(value), true, nil
}

func (cache *RedisCache) setJSON(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode redis metadata cache: %w", err)
	}
	if err := cache.client.Set(ctx, key, data, cache.ttl).Err(); err != nil {
		return fmt.Errorf("set redis metadata cache: %w", err)
	}
	return nil
}

func (cache *RedisCache) invalidatePattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, next, err := cache.client.Scan(ctx, cursor, pattern, cache.scanCount).Result()
		if err != nil {
			return fmt.Errorf("scan redis metadata cache: %w", err)
		}
		if len(keys) > 0 {
			if err := cache.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("delete redis metadata cache keys: %w", err)
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

func (cache *RedisCache) objectKey(bucket, key string) string {
	return cache.prefix + ":object:" + redisKeyPart(bucket) + ":" + redisKeyPart(key)
}

func (cache *RedisCache) bucketsKey() string {
	return cache.prefix + ":" + redisBucketsKey
}

func (cache *RedisCache) listKey(bucket, prefix string) string {
	return cache.prefix + ":list:" + redisKeyPart(bucket) + ":" + redisKeyPart(prefix)
}

func (cache *RedisCache) bucketPattern(bucket string) string {
	return cache.prefix + ":*:" + redisKeyPart(bucket) + ":*"
}

func (cache *RedisCache) listPattern(bucket string) string {
	return cache.prefix + ":list:" + redisKeyPart(bucket) + ":*"
}

func redisKeyPart(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func normalizeRedisPrefix(prefix string) string {
	return strings.Trim(strings.TrimSpace(prefix), ":")
}
