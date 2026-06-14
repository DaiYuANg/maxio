package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/kvx"
	"github.com/redis/go-redis/v9"
)

var errRedisKVUnsupported = errors.New("redis kvx adapter operation is not supported")

type redisKVAdapter struct {
	client RedisClient
}

func newRedisKVAdapter(client RedisClient) kvx.KV {
	return &redisKVAdapter{client: client}
}

func (adapter *redisKVAdapter) Get(ctx context.Context, key string) ([]byte, error) {
	value, err := adapter.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, kvx.ErrNil
	}
	if err != nil {
		return nil, fmt.Errorf("get redis key: %w", err)
	}
	return value, nil
}

func (adapter *redisKVAdapter) MGet(context.Context, []string) (map[string][]byte, error) {
	return nil, errRedisKVUnsupported
}

func (adapter *redisKVAdapter) Set(ctx context.Context, key string, value []byte, expiration time.Duration) error {
	if err := adapter.client.Set(ctx, key, value, expiration).Err(); err != nil {
		return fmt.Errorf("set redis key: %w", err)
	}
	return nil
}

func (adapter *redisKVAdapter) MSet(context.Context, map[string][]byte, time.Duration) error {
	return errRedisKVUnsupported
}

func (adapter *redisKVAdapter) Delete(ctx context.Context, key string) error {
	if err := adapter.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("delete redis key: %w", err)
	}
	return nil
}

func (adapter *redisKVAdapter) DeleteMulti(ctx context.Context, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := adapter.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("delete redis keys: %w", err)
	}
	return nil
}

func (adapter *redisKVAdapter) Exists(context.Context, string) (bool, error) {
	return false, errRedisKVUnsupported
}

func (adapter *redisKVAdapter) ExistsMulti(context.Context, []string) (map[string]bool, error) {
	return nil, errRedisKVUnsupported
}

func (adapter *redisKVAdapter) Expire(context.Context, string, time.Duration) error {
	return errRedisKVUnsupported
}

func (adapter *redisKVAdapter) TTL(context.Context, string) (time.Duration, error) {
	return 0, errRedisKVUnsupported
}

func (adapter *redisKVAdapter) Scan(
	ctx context.Context,
	pattern string,
	cursor uint64,
	count int64,
) (*collectionlist.List[string], uint64, error) {
	keys, next, err := adapter.client.Scan(ctx, cursor, pattern, count).Result()
	if err != nil {
		return nil, 0, fmt.Errorf("scan redis keys: %w", err)
	}
	return collectionlist.NewList(keys...), next, nil
}

func (adapter *redisKVAdapter) Keys(context.Context, string) (*collectionlist.List[string], error) {
	return nil, errRedisKVUnsupported
}
