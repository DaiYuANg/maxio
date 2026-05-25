package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisCall struct {
	name       string
	key        string
	keys       []string
	value      any
	expiration time.Duration
	cursor     uint64
	match      string
	count      int64
}

type scanBatch struct {
	keys   []string
	cursor uint64
	err    error
}

type fakeRedisClient struct {
	values      map[string]string
	calls       []redisCall
	scanBatches []scanBatch
	scanIndex   int
	err         error
	closeErr    error
	closed      bool
}

func newFakeRedisClient() *fakeRedisClient {
	return &fakeRedisClient{values: make(map[string]string)}
}

func (client *fakeRedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	client.calls = append(client.calls, redisCall{name: "get", key: key})
	cmd := redis.NewStringCmd(ctx, "get", key)
	if client.err != nil {
		cmd.SetErr(client.err)
		return cmd
	}
	value, ok := client.values[key]
	if !ok {
		cmd.SetErr(redis.Nil)
		return cmd
	}
	cmd.SetVal(value)
	return cmd
}

func (client *fakeRedisClient) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	client.calls = append(client.calls, redisCall{name: "set", key: key, value: value, expiration: expiration})
	cmd := redis.NewStatusCmd(ctx, "set", key, value)
	if client.err != nil {
		cmd.SetErr(client.err)
		return cmd
	}
	client.values[key] = redisStringValue(value)
	cmd.SetVal("OK")
	return cmd
}

func (client *fakeRedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	client.calls = append(client.calls, redisCall{name: "del", keys: keys})
	cmd := redis.NewIntCmd(ctx, "del", keys)
	if client.err != nil {
		cmd.SetErr(client.err)
		return cmd
	}
	cmd.SetVal(client.deleteKeys(keys))
	return cmd
}

func (client *fakeRedisClient) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	client.calls = append(client.calls, redisCall{name: "scan", cursor: cursor, match: match, count: count})
	cmd := redis.NewScanCmd(ctx, nil, "scan", cursor, "match", match, "count", count)
	if client.err != nil {
		cmd.SetErr(client.err)
		return cmd
	}
	if client.scanIndex >= len(client.scanBatches) {
		cmd.SetVal(nil, 0)
		return cmd
	}
	batch := client.scanBatches[client.scanIndex]
	client.scanIndex++
	if batch.err != nil {
		cmd.SetErr(batch.err)
		return cmd
	}
	cmd.SetVal(batch.keys, batch.cursor)
	return cmd
}

func (client *fakeRedisClient) Close() error {
	client.closed = true
	return client.closeErr
}

func (client *fakeRedisClient) deleteKeys(keys []string) int64 {
	var deleted int64
	for _, key := range keys {
		if _, ok := client.values[key]; ok {
			delete(client.values, key)
			deleted++
		}
	}
	return deleted
}

func redisStringValue(value any) string {
	switch data := value.(type) {
	case []byte:
		return string(data)
	case string:
		return data
	default:
		return ""
	}
}
