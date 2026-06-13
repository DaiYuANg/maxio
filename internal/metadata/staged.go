package metadata

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (m *InMemoryMetadata) ListStagedObjectMetas(_ context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	bucket = strings.TrimSpace(bucket)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.validateOptionalBucketLocked(bucket); err != nil {
		return nil, err
	}

	result := list.NewListWithCapacity[model.ObjectMeta](len(m.staged))
	for key := range m.staged {
		meta := m.staged[key]
		addMatchingStagedObject(result, meta, bucket, prefix)
	}
	sorted := result.Sort(compareObjectLocation)
	return sorted.Values(), nil
}

func (m *InMemoryMetadata) validateOptionalBucketLocked(bucket string) error {
	if bucket == "" {
		return nil
	}
	if _, ok := m.buckets[bucket]; !ok {
		return ErrBucketNotFound
	}
	return nil
}

func addMatchingStagedObject(result *list.List[model.ObjectMeta], meta model.ObjectMeta, bucket, prefix string) {
	if bucket != "" && meta.Bucket != bucket {
		return
	}
	if prefix != "" && !strings.HasPrefix(meta.Key, prefix) {
		return
	}
	result.Add(meta)
}

func compareObjectLocation(left, right model.ObjectMeta) int {
	if left.Bucket < right.Bucket {
		return -1
	}
	if left.Bucket > right.Bucket {
		return 1
	}
	if left.Key < right.Key {
		return -1
	}
	if left.Key > right.Key {
		return 1
	}
	return 0
}

func (m *InMemoryMetadata) StageObjectMeta(_ context.Context, meta model.ObjectMeta) error {
	meta.Bucket = strings.TrimSpace(meta.Bucket)
	meta.Key = strings.TrimSpace(meta.Key)
	meta.Hash = strings.TrimSpace(meta.Hash)
	if meta.Bucket == "" || meta.Key == "" || meta.Hash == "" {
		return ErrBadRequest
	}
	meta.State = model.ObjectStatePending

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.buckets[meta.Bucket]; !ok {
		return ErrBucketNotFound
	}
	m.staged[objectID(meta.Bucket, meta.Key)] = meta
	return nil
}

func (m *InMemoryMetadata) DeleteStagedObjectMeta(_ context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectMeta{}, false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	id := objectID(bucket, key)
	meta, ok := m.staged[id]
	if !ok {
		return model.ObjectMeta{}, false, nil
	}
	delete(m.staged, id)
	return meta, true, nil
}
