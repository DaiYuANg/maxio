package cache

import (
	"context"
	"fmt"
)

func (cache *metadataJSONCache) deletePatternPage(ctx context.Context, pattern string, cursor uint64) (uint64, error) {
	keys, next, err := cache.kv.Scan(ctx, pattern, cursor, cache.scanCount)
	if err != nil {
		return 0, fmt.Errorf("scan metadata cache: %w", err)
	}
	var cacheKeys []string
	if keys != nil {
		cacheKeys = keys.Values()
	}
	if len(cacheKeys) == 0 {
		return next, nil
	}
	if err := cache.kv.DeleteMulti(ctx, cacheKeys); err != nil {
		return 0, fmt.Errorf("delete metadata cache keys: %w", err)
	}
	return next, nil
}
