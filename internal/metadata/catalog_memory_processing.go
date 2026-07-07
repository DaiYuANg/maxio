package metadata

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (m *InMemoryMetadata) UpsertProcessingRecord(_ context.Context, record model.ProcessingRecord) (model.ProcessingRecord, error) {
	record, err := prepareProcessingRecord(record)
	if err != nil {
		return model.ProcessingRecord{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.processingRecords[record.ID]; ok && !existing.CreatedAt.IsZero() {
		record.CreatedAt = existing.CreatedAt
	}
	m.processingRecords[record.ID] = record
	return record, nil
}

func (m *InMemoryMetadata) GetProcessingRecord(_ context.Context, bucket, key, versionID, digest string) (model.ProcessingRecord, bool, error) {
	id := processingRecordID(bucket, key, versionID, digest)
	if id == "" {
		return model.ProcessingRecord{}, false, ErrBadRequest
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	record, found := m.processingRecords[id]
	return record, found, nil
}

func (m *InMemoryMetadata) ListProcessingRecords(_ context.Context, status string, limit int) (*list.List[model.ProcessingRecord], error) {
	status = strings.TrimSpace(status)
	limit = normalizeListLimit(limit)
	m.mu.RLock()
	defer m.mu.RUnlock()
	records := list.NewListWithCapacity[model.ProcessingRecord](len(m.processingRecords))
	for _, record := range m.processingRecords {
		if status == "" || record.Status == status {
			records.Add(record)
		}
	}
	sorted := records.Sort(func(left, right model.ProcessingRecord) int {
		if left.UpdatedAt.After(right.UpdatedAt) {
			return -1
		}
		if left.UpdatedAt.Before(right.UpdatedAt) {
			return 1
		}
		if left.ID < right.ID {
			return -1
		}
		if left.ID > right.ID {
			return 1
		}
		return 0
	})
	if sorted.Len() <= limit {
		return sorted, nil
	}
	return list.NewList(sorted.Values()[:limit]...), nil
}

func (m *InMemoryMetadata) DeleteProcessingRecord(_ context.Context, bucket, key, versionID, digest string) (bool, error) {
	id := processingRecordID(bucket, key, versionID, digest)
	if id == "" {
		return false, ErrBadRequest
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, found := m.processingRecords[id]; !found {
		return false, nil
	}
	delete(m.processingRecords, id)
	return true, nil
}
