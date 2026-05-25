// Package store coordinates metadata and object storage engine operations.
package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/model"
)

var (
	ErrNotFound       = errors.New("object or bucket not found")
	ErrBucketExists   = errors.New("bucket already exists")
	ErrBucketNotFound = errors.New("bucket not found")
	ErrBadRequest     = errors.New("bad request")
	ErrEngineFailed   = errors.New("storage engine failed")
)

// Store is the unified object store: Raft metadata + erasure-coded file storage.
type Store struct {
	meta     metadata.MetadataStore
	engine   *engine.Engine
	recovery recoveryState
}

type blobRefMutation int

const (
	blobRefUnchanged blobRefMutation = iota
	blobRefIncreased
	blobRefCreated
)

func NewStore(dataDir string, meta metadata.MetadataStore, e *engine.Engine) (*Store, error) {
	if meta == nil {
		meta = metadata.NewInMemoryMetadata()
	}
	if e == nil {
		var err error
		e, err = engine.NewEngine(dataDir, engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
		if err != nil {
			return nil, fmt.Errorf("create storage engine: %w", err)
		}
	}
	return &Store{
		meta:   meta,
		engine: e,
	}, nil
}

// --- Bucket operations (metadata only) ---

func (s *Store) ListBuckets(ctx context.Context) ([]model.Bucket, error) {
	buckets, err := s.meta.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", mapStoreError(err))
	}
	return buckets, nil
}

func (s *Store) CreateBucket(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrBadRequest
	}
	return mapStoreError(s.meta.CreateBucket(ctx, name))
}

func (s *Store) DeleteBucket(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrBadRequest
	}
	objects, err := s.meta.ListObjectMetas(ctx, name, "")
	if err != nil {
		return mapStoreError(err)
	}
	for index := range objects {
		meta := objects[index]
		if _, err := s.DeleteObject(ctx, meta.Bucket, meta.Key); err != nil && !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("delete bucket object %q: %w", meta.Key, err)
		}
	}
	return mapStoreError(s.meta.DeleteBucket(ctx, name))
}

// --- Object operations (delegated to engine) ---

func (s *Store) ListObjects(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	bucket = strings.TrimSpace(bucket)
	exists, err := s.meta.BucketExists(ctx, bucket)
	if err != nil {
		return nil, mapStoreError(err)
	}
	if !exists {
		return nil, ErrBucketNotFound
	}
	objects, err := s.meta.ListObjectMetas(ctx, bucket, prefix)
	return objects, mapStoreError(err)
}

func (s *Store) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, model.ObjectMeta, error) {
	meta, exists, err := s.meta.GetObjectMeta(ctx, bucket, key)
	if err != nil {
		return nil, model.ObjectMeta{}, mapStoreError(err)
	}
	if !exists {
		return nil, model.ObjectMeta{}, ErrNotFound
	}

	reader, _, err := s.engine.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, model.ObjectMeta{}, mapStoreError(err)
	}
	return reader, meta, nil
}

func (s *Store) StatObject(ctx context.Context, bucket, key string) (model.ObjectMeta, error) {
	meta, exists, err := s.meta.GetObjectMeta(ctx, bucket, key)
	if err != nil {
		return model.ObjectMeta{}, mapStoreError(err)
	}
	if !exists {
		return model.ObjectMeta{}, ErrNotFound
	}
	return meta, nil
}

func (s *Store) DeleteObject(ctx context.Context, bucket, key string) (model.ObjectMeta, error) {
	objMeta, exists, err := s.meta.GetObjectMeta(ctx, bucket, key)
	if err != nil {
		return model.ObjectMeta{}, mapStoreError(err)
	}
	if !exists {
		return model.ObjectMeta{}, ErrNotFound
	}

	_, _, err = s.meta.DeleteObjectMeta(ctx, bucket, key)
	if err != nil {
		return model.ObjectMeta{}, mapStoreError(err)
	}
	layoutErr := s.engine.DeleteObjectLayout(ctx, bucket, key)
	if err := s.releaseBlob(ctx, objMeta.Hash); err != nil {
		return model.ObjectMeta{}, err
	}
	if layoutErr != nil && !errors.Is(layoutErr, engine.ErrObjectNotFound) {
		return model.ObjectMeta{}, mapStoreError(layoutErr)
	}
	return objMeta, nil
}

// CheckHealth reports shard health for an object.
func (s *Store) CheckHealth(ctx context.Context, bucket, key string) (engine.Health, error) {
	health, err := s.engine.CheckHealth(ctx, bucket, key)
	if err != nil {
		return engine.Health{}, fmt.Errorf("check object health: %w", mapStoreError(err))
	}
	return health, nil
}

// ScrubObject verifies shard checksums and the decoded object checksum.
func (s *Store) ScrubObject(ctx context.Context, bucket, key string) (engine.ScrubResult, error) {
	if _, err := s.StatObject(ctx, bucket, key); err != nil {
		return engine.ScrubResult{}, err
	}
	result, err := s.engine.ScrubObject(ctx, bucket, key)
	if err != nil {
		return engine.ScrubResult{}, fmt.Errorf("scrub object: %w", mapStoreError(err))
	}
	return result, nil
}

// RepairObject reconstructs and writes missing shards for one recoverable object.
func (s *Store) RepairObject(ctx context.Context, bucket, key string) (engine.RepairResult, error) {
	if _, err := s.StatObject(ctx, bucket, key); err != nil {
		return engine.RepairResult{}, err
	}
	result, err := s.engine.RepairObject(ctx, bucket, key)
	if err != nil {
		return engine.RepairResult{}, fmt.Errorf("repair object shards: %w", mapStoreError(err))
	}
	return result, nil
}

func mapStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, metadata.ErrBucketExists) {
		return ErrBucketExists
	}
	if errors.Is(err, metadata.ErrBucketNotFound) {
		return ErrBucketNotFound
	}
	if errors.Is(err, metadata.ErrObjectNotFound) {
		return ErrNotFound
	}
	if errors.Is(err, metadata.ErrBadRequest) {
		return ErrBadRequest
	}
	if errors.Is(err, engine.ErrObjectNotFound) {
		return ErrNotFound
	}
	return err
}
