package cache

import (
	"context"
	"fmt"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func (cache *metadataJSONCache) deletePatternPage(ctx context.Context, pattern string, cursor uint64) (uint64, error) {
	keys, next, err := cache.kv.Scan(ctx, pattern, cursor, cache.scanCount)
	if err != nil {
		return 0, fmt.Errorf("scan metadata cache: %w", err)
	}
	cacheKeys := cacheKeyValues(keys)
	if len(cacheKeys) == 0 {
		return next, nil
	}
	if err := cache.kv.DeleteMulti(ctx, cacheKeys); err != nil {
		return 0, fmt.Errorf("delete metadata cache keys: %w", err)
	}
	return next, nil
}

func cacheKeyValues(keys *collectionlist.List[string]) []string {
	if keys == nil {
		return nil
	}
	var values []string
	keys.ViewValues(func(items []string) {
		values = items
	})
	return values
}
