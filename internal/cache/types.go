// Package cache provides internal composable metadata cache implementations.
package cache

import (
	"context"
	"maps"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/lyonbrown4d/maxio/internal/object"
)

type MetadataCache interface {
	GetBuckets(ctx context.Context) ([]object.Bucket, bool, error)
	SetBuckets(ctx context.Context, buckets []object.Bucket) error
	GetObject(ctx context.Context, bucket, key string) (object.ObjectMeta, bool, error)
	SetObject(ctx context.Context, meta object.ObjectMeta) error
	DeleteObject(ctx context.Context, bucket, key string) error
	GetListObjects(ctx context.Context, bucket, prefix string) ([]object.ObjectMeta, bool, error)
	SetListObjects(ctx context.Context, bucket, prefix string, objects []object.ObjectMeta) error
	GetObjectVersion(ctx context.Context, bucket, key string) (model.ObjectVersion, bool, error)
	SetObjectVersion(ctx context.Context, version model.ObjectVersion) error
	DeleteObjectVersion(ctx context.Context, bucket, key string) error
	GetDigestRef(ctx context.Context, digest string) (model.DigestRef, bool, error)
	SetDigestRef(ctx context.Context, ref model.DigestRef) error
	DeleteDigestRef(ctx context.Context, digest string) error
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

func (c *NoopCache) GetObjectVersion(context.Context, string, string) (model.ObjectVersion, bool, error) {
	return model.ObjectVersion{}, false, nil
}

func (c *NoopCache) SetObjectVersion(context.Context, model.ObjectVersion) error {
	return nil
}

func (c *NoopCache) DeleteObjectVersion(context.Context, string, string) error {
	return nil
}

func (c *NoopCache) GetDigestRef(context.Context, string) (model.DigestRef, bool, error) {
	return model.DigestRef{}, false, nil
}

func (c *NoopCache) SetDigestRef(context.Context, model.DigestRef) error {
	return nil
}

func (c *NoopCache) DeleteDigestRef(context.Context, string) error {
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
	if meta.WriteIntent != nil {
		meta.WriteIntent = new(*meta.WriteIntent)
	}
	return meta
}

func cloneObjectVersion(version model.ObjectVersion) model.ObjectVersion {
	version.UserMetadata = cloneStringMap(version.UserMetadata)
	return version
}

func cloneDigestRef(ref model.DigestRef) model.DigestRef {
	return ref
}

func cloneObjectMetaSlice(items []model.ObjectMeta) []model.ObjectMeta {
	if items == nil {
		return nil
	}
	output := make([]model.ObjectMeta, len(items))
	for i := range items {
		output[i] = cloneObjectMeta(items[i])
	}
	return output
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	maps.Copy(output, input)
	return output
}
