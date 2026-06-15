package metadata

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (m *InMemoryMetadata) ListStagedObjectMetas(_ context.Context, bucket, prefix string) (*list.List[model.ObjectMeta], error) {
	bucket = strings.TrimSpace(bucket)

	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := m.validateOptionalBucketLocked(bucket); err != nil {
		return nil, err
	}

	filtered := list.NewList[model.ObjectMeta]()
	for _, meta := range m.staged {
		if bucket != "" && meta.Bucket != bucket {
			continue
		}
		if prefix != "" && !strings.HasPrefix(meta.Key, prefix) {
			continue
		}
		filtered.Add(meta)
	}
	sorted := filtered.Sort(compareObjectLocation)
	return &sorted, nil
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
