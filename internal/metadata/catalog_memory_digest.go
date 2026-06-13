package metadata

import (
	"context"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func (m *InMemoryMetadata) UpsertDigestRef(_ context.Context, ref model.DigestRef) (model.DigestRef, error) {
	ref, err := prepareMemoryDigestRef(ref)
	if err != nil {
		return model.DigestRef{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.digestRefs[ref.Digest]; ok && !existing.CreatedAt.IsZero() {
		ref.CreatedAt = existing.CreatedAt
	}
	m.digestRefs[ref.Digest] = ref
	return ref, nil
}

func (m *InMemoryMetadata) GetDigestRef(_ context.Context, digest string) (model.DigestRef, bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return model.DigestRef{}, false, ErrBadRequest
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	ref, ok := m.digestRefs[digest]
	if !ok {
		return model.DigestRef{}, false, nil
	}
	return ref, true, nil
}

func (m *InMemoryMetadata) RetainDigestRef(_ context.Context, ref model.DigestRef) (model.DigestRef, error) {
	ref, err := prepareMemoryDigestRef(ref)
	if err != nil {
		return model.DigestRef{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.digestRefs[ref.Digest]; ok {
		ref.CreatedAt = existing.CreatedAt
		ref.RefCount = existing.RefCount + 1
	}
	m.digestRefs[ref.Digest] = ref
	return ref, nil
}

func (m *InMemoryMetadata) ReleaseDigestRef(_ context.Context, digest string) (model.DigestRef, bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return model.DigestRef{}, false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	ref, ok := m.digestRefs[digest]
	if !ok {
		return model.DigestRef{}, false, ErrObjectNotFound
	}
	ref.RefCount--
	ref.UpdatedAt = time.Now().UTC()
	if ref.RefCount <= 0 {
		delete(m.digestRefs, digest)
		return ref, true, nil
	}
	m.digestRefs[digest] = ref
	return ref, false, nil
}

func (m *InMemoryMetadata) DeleteDigestRef(_ context.Context, digest string) (bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.digestRefs[digest]; !ok {
		return false, nil
	}
	delete(m.digestRefs, digest)
	return true, nil
}
