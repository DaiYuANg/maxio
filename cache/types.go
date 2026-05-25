// Package cache provides composable metadata cache implementations.
package cache

import (
	"context"
	"maps"
	"time"

	"github.com/lyonbrown4d/maxio/object"
)

type MetadataCache interface {
	GetBuckets(ctx context.Context) ([]object.Bucket, bool, error)
	SetBuckets(ctx context.Context, buckets []object.Bucket) error
	GetObject(ctx context.Context, bucket, key string) (object.ObjectMeta, bool, error)
	SetObject(ctx context.Context, meta object.ObjectMeta) error
	DeleteObject(ctx context.Context, bucket, key string) error
	GetListObjects(ctx context.Context, bucket, prefix string) ([]object.ObjectMeta, bool, error)
	SetListObjects(ctx context.Context, bucket, prefix string, objects []object.ObjectMeta) error
	InvalidateBucket(ctx context.Context, bucket string) error
	InvalidateAll(ctx context.Context) error
	Close() error
}

type Config struct {
	Backend       string
	TTL           time.Duration
	MaxCost       int64
	RedisAddress  string
	RedisUsername string
	RedisPassword string
	RedisDB       int
	KeyPrefix     string
}

type NoopCache struct{}

func NewNoopCache() *NoopCache {
	return &NoopCache{}
}

func (c *NoopCache) GetBuckets(context.Context) ([]object.Bucket, bool, error) {
	return nil, false, nil
}

func (c *NoopCache) SetBuckets(context.Context, []object.Bucket) error {
	return nil
}

func (c *NoopCache) GetObject(context.Context, string, string) (object.ObjectMeta, bool, error) {
	return object.ObjectMeta{}, false, nil
}

func (c *NoopCache) SetObject(context.Context, object.ObjectMeta) error {
	return nil
}

func (c *NoopCache) DeleteObject(context.Context, string, string) error {
	return nil
}

func (c *NoopCache) GetListObjects(context.Context, string, string) ([]object.ObjectMeta, bool, error) {
	return nil, false, nil
}

func (c *NoopCache) SetListObjects(context.Context, string, string, []object.ObjectMeta) error {
	return nil
}

func (c *NoopCache) InvalidateBucket(context.Context, string) error {
	return nil
}

func (c *NoopCache) InvalidateAll(context.Context) error {
	return nil
}

func (c *NoopCache) Close() error {
	return nil
}

func cloneBuckets(input []object.Bucket) []object.Bucket {
	if len(input) == 0 {
		return nil
	}
	output := make([]object.Bucket, len(input))
	copy(output, input)
	return output
}

func cloneObjectMeta(meta object.ObjectMeta) object.ObjectMeta {
	meta.UserMetadata = cloneStringMap(meta.UserMetadata)
	meta.ShardPlacements = cloneSlice(meta.ShardPlacements)
	meta.ShardChecksums = cloneSlice(meta.ShardChecksums)
	meta.ShardSizes = cloneSlice(meta.ShardSizes)
	if meta.WriteIntent != nil {
		intent := *meta.WriteIntent
		meta.WriteIntent = &intent
	}
	return meta
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	maps.Copy(output, input)
	return output
}

func cloneSlice[T any](input []T) []T {
	if len(input) == 0 {
		return nil
	}
	output := make([]T, len(input))
	copy(output, input)
	return output
}
