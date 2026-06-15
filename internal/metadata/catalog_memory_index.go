package metadata

import (
	"context"
	"sort"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/lo"
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

func (m *InMemoryMetadata) ListIndexDocuments(_ context.Context, bucket, prefix string) ([]model.IndexDocument, error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)

	m.mu.RLock()
	defer m.mu.RUnlock()

	documents := lo.Filter(lo.Values(m.indexDocuments), func(document model.IndexDocument, _ int) bool {
		if bucket != "" && document.Bucket != bucket {
			return false
		}
		if prefix != "" && !strings.HasPrefix(document.Key, prefix) {
			return false
		}
		return true
	})
	sort.Slice(documents, func(i, j int) bool {
		if documents[i].Bucket == documents[j].Bucket {
			return documents[i].Key < documents[j].Key
		}
		return documents[i].Bucket < documents[j].Bucket
	})
	return documents, nil
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

func (m *InMemoryMetadata) ListIndexJobs(_ context.Context, status string, limit int) ([]model.IndexJob, error) {
	status = strings.TrimSpace(status)
	limit = normalizeListLimit(limit)

	m.mu.RLock()
	defer m.mu.RUnlock()

	jobs := lo.Filter(lo.Values(m.indexJobs), func(job model.IndexJob, _ int) bool {
		return status == "" || job.Status == status
	})
	if queryLimit := limit; queryLimit > 0 && len(jobs) > queryLimit {
		jobs = lo.Take(jobs, queryLimit)
	}
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
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

func (m *InMemoryMetadata) ListIndexOutboxEvents(_ context.Context, status string, limit int) ([]model.IndexOutboxEvent, error) {
	status = strings.TrimSpace(status)
	limit = normalizeListLimit(limit)

	m.mu.RLock()
	defer m.mu.RUnlock()

	events := lo.Filter(lo.Values(m.indexOutbox), func(event model.IndexOutboxEvent, _ int) bool {
		return status == "" || event.Status == status
	})
	if queryLimit := limit; queryLimit > 0 && len(events) > queryLimit {
		events = lo.Take(events, queryLimit)
	}
	sort.Slice(events, func(i, j int) bool {
		return events[i].CreatedAt.Before(events[j].CreatedAt)
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
