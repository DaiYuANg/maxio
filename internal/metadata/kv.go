// Package metadata provides metadata persistence abstractions and implementations
// for object/bucket/index state used by maxio.
package metadata

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/model"
)

var (
	ErrBadRequest     = errors.New("bad request")
	ErrBucketExists   = errors.New("bucket already exists")
	ErrBucketNotFound = errors.New("bucket not found")
	ErrObjectNotFound = errors.New("object not found")
)

type BlobRef struct {
	Hash            string
	Path            string
	ShardPlacements []model.ShardPlacement
	ShardChecksums  []string
	ShardSizes      []int64
	RefCount        int
	Size            int64
}

type Repository interface {
	ListBuckets(ctx context.Context) ([]model.Bucket, error)
	BucketExists(ctx context.Context, bucket string) (bool, error)
	CreateBucket(ctx context.Context, bucket string) error
	DeleteBucket(ctx context.Context, bucket string) error

	ListObjectMetas(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error)
	ListStagedObjectMetas(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error)
	GetObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error)
	StageObjectMeta(ctx context.Context, meta model.ObjectMeta) error
	UpsertObjectMeta(ctx context.Context, meta model.ObjectMeta) error
	DeleteStagedObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error)
	DeleteObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error)

	ListBlobRefs(ctx context.Context) ([]BlobRef, error)
	GetBlobRef(ctx context.Context, hash string) (BlobRef, bool, error)
	CreateBlobRef(ctx context.Context, hash, path string, size int64, placements []model.ShardPlacement, checksums []string, shardSizes ...[]int64) error
	UpdateBlobRefPlacements(ctx context.Context, hash string, placements []model.ShardPlacement) error
	IncreaseBlobRef(ctx context.Context, hash string) error
	DecreaseBlobRef(ctx context.Context, hash string) (string, bool, error)
}

type MetadataStore = Repository

type InMemoryMetadata struct {
	mu      sync.RWMutex
	buckets map[string]map[string]struct{}
	objects map[string]model.ObjectMeta
	staged  map[string]model.ObjectMeta
	blobs   map[string]BlobRef
}

func NewInMemoryMetadata() *InMemoryMetadata {
	return &InMemoryMetadata{
		buckets: make(map[string]map[string]struct{}),
		objects: make(map[string]model.ObjectMeta),
		staged:  make(map[string]model.ObjectMeta),
		blobs:   make(map[string]BlobRef),
	}
}

func (m *InMemoryMetadata) ListBuckets(context.Context) ([]model.Bucket, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now().UTC()
	buckets := list.NewListWithCapacity[model.Bucket](len(m.buckets))
	for name := range m.buckets {
		buckets.Add(model.Bucket{
			Name:      name,
			CreatedAt: now,
		})
	}
	sorted := buckets.Sort(func(left, right model.Bucket) int {
		if left.Name < right.Name {
			return -1
		}
		if left.Name > right.Name {
			return 1
		}
		return 0
	})
	return sorted.Values(), nil
}

func (m *InMemoryMetadata) BucketExists(_ context.Context, bucket string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return false, ErrBadRequest
	}
	_, ok := m.buckets[bucket]
	return ok, nil
}

func (m *InMemoryMetadata) CreateBucket(_ context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.buckets[bucket]; ok {
		return ErrBucketExists
	}
	m.buckets[bucket] = make(map[string]struct{})
	return nil
}

func (m *InMemoryMetadata) DeleteBucket(_ context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	keys, ok := m.buckets[bucket]
	if !ok {
		return ErrBucketNotFound
	}
	for key := range keys {
		id := objectID(bucket, key)
		meta := m.objects[id]
		delete(m.objects, id)
		if _, _, err := m.decreaseBlobRefLocked(meta.Hash); err != nil && !errors.Is(err, ErrObjectNotFound) {
			return err
		}
	}
	delete(m.buckets, bucket)
	for key := range m.staged {
		meta := m.staged[key]
		if meta.Bucket == bucket {
			delete(m.staged, key)
		}
	}
	return nil
}

func (m *InMemoryMetadata) ListObjectMetas(_ context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, ErrBadRequest
	}
	keys, ok := m.buckets[bucket]
	if !ok {
		return nil, ErrBucketNotFound
	}

	result := list.NewListWithCapacity[model.ObjectMeta](len(keys))
	for key := range keys {
		if strings.HasPrefix(key, prefix) {
			meta := m.objects[objectID(bucket, key)]
			result.Add(meta)
		}
	}
	sorted := result.Sort(func(left, right model.ObjectMeta) int {
		if left.Key < right.Key {
			return -1
		}
		if left.Key > right.Key {
			return 1
		}
		return 0
	})
	return sorted.Values(), nil
}

func (m *InMemoryMetadata) GetObjectMeta(_ context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectMeta{}, false, ErrBadRequest
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	meta, ok := m.objects[objectID(bucket, key)]
	if !ok {
		return model.ObjectMeta{}, false, nil
	}
	return meta, true, nil
}

func (m *InMemoryMetadata) UpsertObjectMeta(_ context.Context, meta model.ObjectMeta) error {
	meta.Bucket = strings.TrimSpace(meta.Bucket)
	meta.Key = strings.TrimSpace(meta.Key)
	if meta.Bucket == "" || meta.Key == "" {
		return ErrBadRequest
	}
	meta.State = model.ObjectStateCommitted

	m.mu.Lock()
	defer m.mu.Unlock()
	keys, ok := m.buckets[meta.Bucket]
	if !ok {
		return ErrBucketNotFound
	}
	keys[meta.Key] = struct{}{}
	m.objects[objectID(meta.Bucket, meta.Key)] = meta
	return nil
}

func (m *InMemoryMetadata) DeleteObjectMeta(_ context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectMeta{}, false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := objectID(bucket, key)
	meta, ok := m.objects[id]
	if !ok {
		return model.ObjectMeta{}, false, nil
	}
	delete(m.objects, id)
	if keys, ok := m.buckets[bucket]; ok {
		delete(keys, key)
	}
	return meta, true, nil
}

func objectID(bucket, key string) string {
	return bucket + "\x00" + key
}
