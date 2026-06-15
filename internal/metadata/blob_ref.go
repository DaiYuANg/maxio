package metadata

import (
	"context"

	"github.com/arcgolabs/collectionx/list"
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
	refs := list.NewList[BlobRef]()
	for hash, ref := range m.blobs {
		ref.Hash = hash
		refs.Add(ref)
	}
	return refs.Values(), nil
}

func (m *InMemoryMetadata) CreateBlobRef(
	_ context.Context,
	hash string,
	path string,
	size int64,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.blobs[hash]; ok {
		return nil
	}
	m.blobs[hash] = BlobRef{
		Hash:     hash,
		Path:     path,
		RefCount: 1,
		Size:     size,
	}
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
