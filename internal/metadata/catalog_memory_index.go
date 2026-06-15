package metadata

import (
	"context"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (m *InMemoryMetadata) UpsertIndexDocument(_ context.Context, document model.IndexDocument) (model.IndexDocument, error) {
	document, err := prepareIndexDocument(document)
	if err != nil {
		return model.IndexDocument{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.indexDocuments[document.ID]; ok && !existing.CreatedAt.IsZero() {
		document.CreatedAt = existing.CreatedAt
	}
	m.indexDocuments[document.ID] = document
	return document, nil
}

func (m *InMemoryMetadata) GetIndexDocument(_ context.Context, id string) (model.IndexDocument, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexDocument{}, false, ErrBadRequest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	document, ok := m.indexDocuments[id]
	if !ok {
		return model.IndexDocument{}, false, nil
	}
	return document, true, nil
}

func (m *InMemoryMetadata) ListIndexDocuments(_ context.Context, bucket, prefix string) (*list.List[model.IndexDocument], error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)

	m.mu.RLock()
	defer m.mu.RUnlock()

	sorted := list.FilterMapList(
		listValuesFromMap(m.indexDocuments),
		func(_ int, document model.IndexDocument) (model.IndexDocument, bool) {
			if bucket != "" && document.Bucket != bucket {
				return model.IndexDocument{}, false
			}
			if prefix != "" && !strings.HasPrefix(document.Key, prefix) {
				return model.IndexDocument{}, false
			}
			return document, true
		},
	).Sort(func(left, right model.IndexDocument) int {
		if left.Bucket == right.Bucket {
			switch {
			case left.Key < right.Key:
				return -1
			case left.Key > right.Key:
				return 1
			default:
				return 0
			}
		}
		switch {
		case left.Bucket < right.Bucket:
			return -1
		case left.Bucket > right.Bucket:
			return 1
		default:
			return 0
		}
	})
	return sorted, nil
}

func (m *InMemoryMetadata) DeleteIndexDocument(_ context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.indexDocuments[id]; !ok {
		return false, nil
	}
	delete(m.indexDocuments, id)
	return true, nil
}

func (m *InMemoryMetadata) UpsertIndexJob(_ context.Context, job model.IndexJob) (model.IndexJob, error) {
	job, err := prepareIndexJob(job)
	if err != nil {
		return model.IndexJob{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.indexJobs[job.ID]; ok && !existing.CreatedAt.IsZero() {
		job.CreatedAt = existing.CreatedAt
	}
	m.indexJobs[job.ID] = job
	return job, nil
}

func (m *InMemoryMetadata) GetIndexJob(_ context.Context, id string) (model.IndexJob, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexJob{}, false, ErrBadRequest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	job, ok := m.indexJobs[id]
	if !ok {
		return model.IndexJob{}, false, nil
	}
	return job, true, nil
}

func (m *InMemoryMetadata) ListIndexJobs(_ context.Context, status string, limit int) (*list.List[model.IndexJob], error) {
	status = strings.TrimSpace(status)
	limit = normalizeListLimit(limit)

	m.mu.RLock()
	defer m.mu.RUnlock()

	jobs := list.FilterMapList(
		listValuesFromMap(m.indexJobs),
		func(_ int, job model.IndexJob) (model.IndexJob, bool) {
			if status != "" && job.Status != status {
				return model.IndexJob{}, false
			}
			return job, true
		},
	)
	if queryLimit := limit; queryLimit > 0 && jobs.Len() > queryLimit {
		jobs = jobs.Take(queryLimit)
	}
	jobs = jobs.Sort(func(left, right model.IndexJob) int {
		switch {
		case left.CreatedAt.Before(right.CreatedAt):
			return -1
		case left.CreatedAt.After(right.CreatedAt):
			return 1
		default:
			return 0
		}
	})
	return jobs, nil
}

func (m *InMemoryMetadata) DeleteIndexJob(_ context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.indexJobs[id]; !ok {
		return false, nil
	}
	delete(m.indexJobs, id)
	return true, nil
}

func (m *InMemoryMetadata) UpsertIndexOutboxEvent(_ context.Context, event model.IndexOutboxEvent) (model.IndexOutboxEvent, error) {
	event, err := prepareIndexOutboxEvent(event)
	if err != nil {
		return model.IndexOutboxEvent{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.indexOutbox[event.ID]; ok && !existing.CreatedAt.IsZero() {
		event.CreatedAt = existing.CreatedAt
	}
	m.indexOutbox[event.ID] = event
	return event, nil
}

func (m *InMemoryMetadata) GetIndexOutboxEvent(_ context.Context, id string) (model.IndexOutboxEvent, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexOutboxEvent{}, false, ErrBadRequest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	event, ok := m.indexOutbox[id]
	if !ok {
		return model.IndexOutboxEvent{}, false, nil
	}
	return event, true, nil
}

func (m *InMemoryMetadata) ListIndexOutboxEvents(_ context.Context, status string, limit int) (*list.List[model.IndexOutboxEvent], error) {
	status = strings.TrimSpace(status)
	limit = normalizeListLimit(limit)

	m.mu.RLock()
	defer m.mu.RUnlock()

	events := list.FilterMapList(
		listValuesFromMap(m.indexOutbox),
		func(_ int, event model.IndexOutboxEvent) (model.IndexOutboxEvent, bool) {
			if status != "" && event.Status != status {
				return model.IndexOutboxEvent{}, false
			}
			return event, true
		},
	)
	if queryLimit := limit; queryLimit > 0 && events.Len() > queryLimit {
		events = events.Take(queryLimit)
	}
	events = events.Sort(func(left, right model.IndexOutboxEvent) int {
		switch {
		case left.CreatedAt.Before(right.CreatedAt):
			return -1
		case left.CreatedAt.After(right.CreatedAt):
			return 1
		default:
			return 0
		}
	})
	return events, nil
}

func (m *InMemoryMetadata) DeleteIndexOutboxEvent(_ context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.indexOutbox[id]; !ok {
		return false, nil
	}
	delete(m.indexOutbox, id)
	return true, nil
}
