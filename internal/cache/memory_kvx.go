package cache

import (
	"context"
	"errors"
	"fmt"
	"path"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/mapping"
	"github.com/arcgolabs/kvx"
	"github.com/dgraph-io/ristretto"
	"github.com/samber/lo"
)

const memoryKVKeyRow = "keys"
const maxMemoryScanPageSize = uint64(1_000_000)

var errMemoryKVUnsupported = errors.New("memory kvx adapter operation is not supported")

type memoryKVAdapter struct {
	cache *ristretto.Cache
	keys  *mapping.ConcurrentTable[string, string, struct{}]
}

func newMemoryKVAdapter(cache *ristretto.Cache) *memoryKVAdapter {
	return &memoryKVAdapter{
		cache: cache,
		keys:  mapping.NewConcurrentTable[string, string, struct{}](),
	}
}

func (adapter *memoryKVAdapter) Get(_ context.Context, key string) ([]byte, error) {
	value, ok := adapter.cache.Get(key)
	if !ok {
		return nil, kvx.ErrNil
	}
	data, ok := value.([]byte)
	if !ok {
		adapter.cache.Del(key)
		adapter.keys.Delete(memoryKVKeyRow, key)
		return nil, kvx.ErrNil
	}
	return cloneBytes(data), nil
}

func (adapter *memoryKVAdapter) MGet(context.Context, []string) (map[string][]byte, error) {
	return nil, errMemoryKVUnsupported
}

func (adapter *memoryKVAdapter) Set(_ context.Context, key string, value []byte, expiration time.Duration) error {
	data := cloneBytes(value)
	if adapter.cache.SetWithTTL(key, data, int64(len(data)), expiration) {
		adapter.keys.Put(memoryKVKeyRow, key, struct{}{})
	}
	return nil
}

func (adapter *memoryKVAdapter) MSet(context.Context, map[string][]byte, time.Duration) error {
	return errMemoryKVUnsupported
}

func (adapter *memoryKVAdapter) Delete(_ context.Context, key string) error {
	adapter.cache.Del(key)
	adapter.keys.Delete(memoryKVKeyRow, key)
	return nil
}

func (adapter *memoryKVAdapter) DeleteMulti(_ context.Context, keys []string) error {
	for _, key := range keys {
		adapter.cache.Del(key)
		adapter.keys.Delete(memoryKVKeyRow, key)
	}
	return nil
}

func (adapter *memoryKVAdapter) Exists(context.Context, string) (bool, error) {
	return false, errMemoryKVUnsupported
}

func (adapter *memoryKVAdapter) ExistsMulti(context.Context, []string) (map[string]bool, error) {
	return nil, errMemoryKVUnsupported
}

func (adapter *memoryKVAdapter) Expire(context.Context, string, time.Duration) error {
	return errMemoryKVUnsupported
}

func (adapter *memoryKVAdapter) TTL(context.Context, string) (time.Duration, error) {
	return 0, errMemoryKVUnsupported
}

func (adapter *memoryKVAdapter) Scan(
	_ context.Context,
	pattern string,
	cursor uint64,
	count int64,
) (*collectionlist.List[string], uint64, error) {
	matched, err := adapter.scanKeys(pattern)
	if err != nil {
		return nil, 0, err
	}
	return pageScanKeys(matched, cursor, count)
}

func (adapter *memoryKVAdapter) Keys(_ context.Context, pattern string) (*collectionlist.List[string], error) {
	matched, err := adapter.scanKeys(pattern)
	if err != nil {
		return nil, err
	}
	return collectionlist.NewList(matched...), nil
}

func (adapter *memoryKVAdapter) Close() error {
	adapter.cache.Close()
	adapter.keys.Clear()
	return nil
}

func (adapter *memoryKVAdapter) Wait() {
	adapter.cache.Wait()
}

func (adapter *memoryKVAdapter) scanKeys(pattern string) ([]string, error) {
	row := adapter.keys.Row(memoryKVKeyRow)
	keys := lo.Keys(row)
	matched := make([]string, 0, len(keys))
	for _, key := range keys {
		ok, err := path.Match(pattern, key)
		if err != nil {
			return nil, fmt.Errorf("match memory cache key pattern: %w", err)
		}
		if ok {
			matched = append(matched, key)
		}
	}
	return matched, nil
}

func pageScanKeys(keys []string, cursor uint64, count int64) (*collectionlist.List[string], uint64, error) {
	limit, limited := memoryScanLimit(count)
	result := make([]string, 0)
	var current uint64
	var taken uint64
	for _, key := range keys {
		if current >= cursor {
			if limited && taken >= limit {
				return collectionlist.NewList(result...), current, nil
			}
			result = append(result, key)
			taken++
		}
		current++
	}
	return collectionlist.NewList(result...), 0, nil
}

func memoryScanLimit(count int64) (uint64, bool) {
	if count <= 0 {
		return 0, false
	}
	limit := uint64(0)
	for count > 0 && limit < maxMemoryScanPageSize {
		limit++
		count--
	}
	return limit, true
}

func cloneBytes(input []byte) []byte {
	if len(input) == 0 {
		return nil
	}
	output := make([]byte, len(input))
	copy(output, input)
	return output
}
