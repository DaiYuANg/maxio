package metadata

import (
	"context"
	"sort"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/lo"
)

func (m *InMemoryMetadata) UpsertObjectRecord(_ context.Context, record model.ObjectRecord) (model.ObjectRecord, error) {
	record, err := prepareMemoryObjectRecord(record)
	if err != nil {
		return model.ObjectRecord{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.buckets[record.Bucket]; !ok {
		return model.ObjectRecord{}, ErrBucketNotFound
	}
	id := objectID(record.Bucket, record.Key)
	if existing, ok := m.objectRecords[id]; ok && !existing.CreatedAt.IsZero() {
		record.CreatedAt = existing.CreatedAt
	}
	m.objectRecords[id] = record
	return cloneObjectRecord(record), nil
}

func (m *InMemoryMetadata) GetObjectRecord(_ context.Context, bucket, key string) (model.ObjectRecord, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectRecord{}, false, ErrBadRequest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	record, ok := m.objectRecords[objectID(bucket, key)]
	if !ok {
		return model.ObjectRecord{}, false, nil
	}
	return cloneObjectRecord(record), true, nil
}

func (m *InMemoryMetadata) DeleteObjectRecord(_ context.Context, bucket, key string) (bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := objectID(bucket, key)
	if _, ok := m.objectRecords[id]; !ok {
		return false, nil
	}
	delete(m.objectRecords, id)
	return true, nil
}

func (m *InMemoryMetadata) UpsertObjectVersion(_ context.Context, version model.ObjectVersion) (model.ObjectVersion, error) {
	version, err := prepareMemoryObjectVersion(version)
	if err != nil {
		return model.ObjectVersion{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.buckets[version.Bucket]; !ok {
		return model.ObjectVersion{}, ErrBucketNotFound
	}
	id := objectVersionID(version.Bucket, version.Key, version.VersionID)
	if existing, ok := m.objectVersions[id]; ok && !existing.CreatedAt.IsZero() {
		version.CreatedAt = existing.CreatedAt
	}
	m.objectVersions[id] = cloneObjectVersion(version)
	return cloneObjectVersion(version), nil
}

func (m *InMemoryMetadata) GetObjectVersion(_ context.Context, bucket, key, versionID string) (model.ObjectVersion, bool, error) {
	bucket, key, versionID = trimObjectVersionKey(bucket, key, versionID)
	if bucket == "" || key == "" || versionID == "" {
		return model.ObjectVersion{}, false, ErrBadRequest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	version, ok := m.objectVersions[objectVersionID(bucket, key, versionID)]
	if !ok {
		return model.ObjectVersion{}, false, nil
	}
	return cloneObjectVersion(version), true, nil
}

func (m *InMemoryMetadata) ListObjectVersions(_ context.Context, bucket, key string) ([]model.ObjectVersion, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return nil, ErrBadRequest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	versions := lo.FilterMap(lo.Values(m.objectVersions), func(version model.ObjectVersion, _ int) (model.ObjectVersion, bool) {
		if version.Bucket != bucket || version.Key != key {
			return model.ObjectVersion{}, false
		}
		return cloneObjectVersion(version), true
	})
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].CreatedAt.After(versions[j].CreatedAt)
	})
	return versions, nil
}

func (m *InMemoryMetadata) DeleteObjectVersion(_ context.Context, bucket, key, versionID string) (bool, error) {
	bucket, key, versionID = trimObjectVersionKey(bucket, key, versionID)
	if bucket == "" || key == "" || versionID == "" {
		return false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := objectVersionID(bucket, key, versionID)
	if _, ok := m.objectVersions[id]; !ok {
		return false, nil
	}
	delete(m.objectVersions, id)
	return true, nil
}
