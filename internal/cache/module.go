package cache

import (
	"context"
	"fmt"
	"strings"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/redis/go-redis/v9"
)

func Module() dix.Module {
	return dix.NewModule(
		"cache",
		dix.WithModuleProviders(
			dix.ProviderErr1(newMetadataCacheFromRuntimeConfig),
		),
		dix.Hooks(
			dix.OnStop(func(_ context.Context, metadata MetadataCache) error {
				if metadata == nil {
					return nil
				}
				return metadata.Close()
			}),
		),
	)
}

func newMetadataCacheFromRuntimeConfig(cfg config.Config) (MetadataCache, error) {
	cacheConfig := Config{
		Backend:       cfg.CacheBackend,
		TTL:           cfg.CacheTTLDuration(),
		MaxCost:       int64(cfg.CacheMaxCost),
		RedisAddress:  cfg.CacheRedisAddress,
		RedisUsername: cfg.CacheRedisUsername,
		RedisPassword: cfg.CacheRedisPassword,
		RedisDB:       cfg.CacheRedisDB,
		KeyPrefix:     cfg.CacheKeyPrefix,
	}
	switch strings.ToLower(strings.TrimSpace(cacheConfig.Backend)) {
	case "", "none":
		return NewNoopCache(), nil
	case "memory":
		return NewMemoryMetadataCache(MemoryConfig{TTL: cacheConfig.TTL, MaxCost: cacheConfig.MaxCost})
	case "redis":
		return NewRedisCache(
			redis.NewClient(&redis.Options{
				Addr:     cacheConfig.RedisAddress,
				Username: cacheConfig.RedisUsername,
				Password: cacheConfig.RedisPassword,
				DB:       cacheConfig.RedisDB,
			}),
			WithRedisPrefix(cacheConfig.KeyPrefix),
			WithRedisTTL(cacheConfig.TTL),
		), nil
	default:
		return nil, fmt.Errorf("unsupported cache backend %q", cacheConfig.Backend)
	}
}
