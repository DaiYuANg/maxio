package metadata

import (
	"context"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/model"
)

func (m *InMemoryMetadata) ListUpstreams(context.Context) ([]model.Upstream, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	upstreams := list.NewListWithCapacity[model.Upstream](len(m.upstreams))
	for id := range m.upstreams {
		upstream := m.upstreams[id]
		upstreams.Add(cloneUpstream(upstream))
	}
	sorted := upstreams.Sort(compareUpstream)
	return sorted.Values(), nil
}

func (m *InMemoryMetadata) GetUpstream(_ context.Context, id string) (model.Upstream, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.Upstream{}, false, ErrBadRequest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	upstream, ok := m.upstreams[id]
	if !ok {
		return model.Upstream{}, false, nil
	}
	return cloneUpstream(upstream), true, nil
}

func (m *InMemoryMetadata) UpsertUpstream(_ context.Context, upstream model.Upstream) (model.Upstream, error) {
	upstream, err := normalizeUpstream(upstream)
	if err != nil {
		return model.Upstream{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.upstreams[upstream.ID]; ok && !existing.CreatedAt.IsZero() {
		upstream.CreatedAt = existing.CreatedAt
	}
	now := time.Now().UTC()
	if upstream.CreatedAt.IsZero() {
		upstream.CreatedAt = now
	}
	upstream.UpdatedAt = now
	m.upstreams[upstream.ID] = cloneUpstream(upstream)
	return cloneUpstream(upstream), nil
}

func (m *InMemoryMetadata) DeleteUpstream(_ context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.upstreams[id]; !ok {
		return false, nil
	}
	delete(m.upstreams, id)
	return true, nil
}

func compareUpstream(left, right model.Upstream) int {
	if left.Priority < right.Priority {
		return -1
	}
	if left.Priority > right.Priority {
		return 1
	}
	if left.Name < right.Name {
		return -1
	}
	if left.Name > right.Name {
		return 1
	}
	if left.ID < right.ID {
		return -1
	}
	if left.ID > right.ID {
		return 1
	}
	return 0
}

func cloneUpstream(upstream model.Upstream) model.Upstream {
	upstream.Buckets = append([]string(nil), upstream.Buckets...)
	return upstream
}
