// Package cache provides metadata cache implementations for MaxIO.
package cache

import (
	"fmt"
	"time"

	"github.com/dgraph-io/ristretto"
)

const (
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
	*metadataJSONCache
	kv *memoryKVAdapter
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
	kv := newMemoryKVAdapter(rc)
	return &MemoryMetadataCache{
		metadataJSONCache: newMetadataJSONCache(kv, defaultCachePrefix, cfg.TTL, defaultCacheScanCount),
		kv:                kv,
	}, nil
}

// Close releases cache resources.
func (c *MemoryMetadataCache) Close() error {
	if c == nil || c.kv == nil {
		return nil
	}
	return c.kv.Close()
}

// Wait blocks until pending Ristretto writes have been processed.
func (c *MemoryMetadataCache) Wait() {
	if c == nil || c.kv == nil {
		return
	}
	c.kv.Wait()
}
