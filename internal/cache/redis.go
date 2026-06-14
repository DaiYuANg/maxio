// Package cache contains metadata cache implementations.
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
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
	*metadataJSONCache
	client RedisClient
}

// RedisOption configures RedisCache.
type RedisOption func(*redisCacheOptions)

type redisCacheOptions struct {
	prefix    string
	ttl       time.Duration
	scanCount int64
}

// WithRedisPrefix sets the Redis key prefix.
func WithRedisPrefix(prefix string) RedisOption {
	return func(options *redisCacheOptions) {
		options.prefix = prefix
	}
}

// WithRedisTTL sets the cache TTL.
func WithRedisTTL(ttl time.Duration) RedisOption {
	return func(options *redisCacheOptions) {
		options.ttl = ttl
	}
}

// WithRedisScanCount sets the SCAN batch size used by invalidation.
func WithRedisScanCount(count int64) RedisOption {
	return func(options *redisCacheOptions) {
		options.scanCount = count
	}
}

// NewRedisCache creates a Redis-backed metadata cache.
func NewRedisCache(client RedisClient, opts ...RedisOption) *RedisCache {
	options := redisCacheOptions{
		prefix:    defaultCachePrefix,
		ttl:       defaultCacheTTL,
		scanCount: defaultCacheScanCount,
	}
	for _, opt := range opts {
		opt(&options)
	}
	return &RedisCache{
		metadataJSONCache: newMetadataJSONCache(
			newRedisKVAdapter(client),
			options.prefix,
			options.ttl,
			options.scanCount,
		),
		client: client,
	}
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
