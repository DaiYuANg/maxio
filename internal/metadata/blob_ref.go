package metadata

import (
	"context"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func (m *InMemoryMetadata) GetBlobRef(_ context.Context, hash string) (BlobRef, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ref, ok := m.blobs[hash]
	ref.Hash = hash
	return ref, ok, nil
}

func (m *InMemoryMetadata) ListBlobRefs(_ context.Context) ([]BlobRef, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	refs := make([]BlobRef, 0, len(m.blobs))
	for hash, ref := range m.blobs {
		ref.Hash = hash
		ref.ShardPlacements = cloneBlobRefPlacements(ref.ShardPlacements)
		ref.ShardChecksums = cloneStrings(ref.ShardChecksums)
		ref.ShardSizes = cloneInt64s(ref.ShardSizes)
		refs = append(refs, ref)
	}
	return refs, nil
}

func (m *InMemoryMetadata) CreateBlobRef(
	_ context.Context,
	hash string,
	path string,
	size int64,
	placements []model.ShardPlacement,
	checksums []string,
	shardSizes ...[]int64,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.blobs[hash]; ok {
		return nil
	}
	m.blobs[hash] = BlobRef{
		Hash:            hash,
		Path:            path,
		ShardPlacements: cloneBlobRefPlacements(placements),
		ShardChecksums:  cloneStrings(checksums),
		ShardSizes:      cloneFirstInt64s(shardSizes),
		RefCount:        1,
		Size:            size,
	}
	return nil
}

func (m *InMemoryMetadata) UpdateBlobRefPlacements(_ context.Context, hash string, placements []model.ShardPlacement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.blobs[hash]
	if !ok {
		return ErrObjectNotFound
	}
	ref.ShardPlacements = cloneBlobRefPlacements(placements)
	m.blobs[hash] = ref
	return nil
}

func (m *InMemoryMetadata) IncreaseBlobRef(_ context.Context, hash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.blobs[hash]
	if !ok {
		return ErrObjectNotFound
	}
	ref.RefCount++
	m.blobs[hash] = ref
	return nil
}

func (m *InMemoryMetadata) DecreaseBlobRef(_ context.Context, hash string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.decreaseBlobRefLocked(hash)
}

func (m *InMemoryMetadata) decreaseBlobRefLocked(hash string) (string, bool, error) {
	ref, ok := m.blobs[hash]
	if !ok {
		return "", false, ErrObjectNotFound
	}
	ref.RefCount--
	if ref.RefCount <= 0 {
		delete(m.blobs, hash)
		return ref.Path, true, nil
	}
	m.blobs[hash] = ref
	return ref.Path, false, nil
}
