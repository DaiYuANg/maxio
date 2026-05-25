package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/model"
)

func (s *Store) prepareBlob(
	ctx context.Context,
	key string,
	staged *stagedObject,
) (engine.BlobInfo, bool, bool, error) {
	if staged == nil {
		return engine.BlobInfo{}, false, false, ErrBadRequest
	}
	ref, exists, err := s.meta.GetBlobRef(ctx, staged.hash)
	if err != nil {
		return engine.BlobInfo{}, false, false, mapStoreError(err)
	}
	if exists {
		return engine.BlobInfo{
			Hash:            staged.hash,
			ETag:            engine.ETagFromHash(staged.hash),
			Size:            ref.Size,
			ShardDir:        ref.Path,
			ShardPlacements: ref.ShardPlacements,
			ShardChecksums:  ref.ShardChecksums,
			ShardSizes:      ref.ShardSizes,
		}, true, false, nil
	}
	return s.createBlob(ctx, key, staged)
}

func (s *Store) createBlob(ctx context.Context, key string, staged *stagedObject) (engine.BlobInfo, bool, bool, error) {
	blobReader, err := staged.Reader()
	if err != nil {
		return engine.BlobInfo{}, false, false, fmt.Errorf("%w: stage object input: %w", ErrEngineFailed, err)
	}
	blob, err := s.engine.PutBlob(ctx, key, blobReader)
	if err != nil {
		return engine.BlobInfo{}, false, false, fmt.Errorf("%w: %w", ErrEngineFailed, err)
	}
	if blob.Hash != staged.hash {
		return engine.BlobInfo{}, false, true, fmt.Errorf("%w: staged object hash mismatch", ErrEngineFailed)
	}
	return blob, false, true, nil
}

func (s *Store) retainBlob(
	ctx context.Context,
	hash string,
	blob engine.BlobInfo,
	refExists bool,
	sameObjectBlob bool,
) (blobRefMutation, error) {
	if !refExists {
		if err := s.meta.CreateBlobRef(ctx, hash, blob.ShardDir, blob.Size, blob.ShardPlacements, blob.ShardChecksums, blob.ShardSizes); err != nil {
			return blobRefUnchanged, mapStoreError(err)
		}
		return blobRefCreated, nil
	}
	if sameObjectBlob {
		return blobRefUnchanged, nil
	}
	if err := s.meta.IncreaseBlobRef(ctx, hash); err != nil {
		return blobRefUnchanged, mapStoreError(err)
	}
	return blobRefIncreased, nil
}

func (s *Store) rollbackPut(
	ctx context.Context,
	bucket string,
	key string,
	hash string,
	blob engine.BlobInfo,
	mutation blobRefMutation,
	createdBlob bool,
	existing existingObject,
) error {
	err := s.deletePutLayout(ctx, bucket, key)
	err = errors.Join(err, s.deleteStagedObjectMeta(ctx, bucket, key))
	err = errors.Join(err, s.restorePreviousLayout(ctx, bucket, key, existing))
	err = errors.Join(err, s.rollbackBlobRef(ctx, hash, mutation))
	err = errors.Join(err, s.cleanupCreatedBlob(ctx, blob, createdBlob))
	return err
}

func (s *Store) deletePutLayout(ctx context.Context, bucket, key string) error {
	if err := s.engine.DeleteObjectLayout(ctx, bucket, key); err != nil {
		return fmt.Errorf("delete put layout: %w", err)
	}
	return nil
}

func (s *Store) restorePreviousLayout(ctx context.Context, bucket, key string, existing existingObject) error {
	if !existing.exists || !existing.refExists {
		return nil
	}
	_, err := s.engine.LinkObject(ctx, bucket, key, engine.BlobInfo{
		Hash:            existing.meta.Hash,
		ETag:            existing.meta.ETag,
		Size:            existing.ref.Size,
		ShardDir:        existing.ref.Path,
		ShardPlacements: existing.ref.ShardPlacements,
		ShardChecksums:  existing.ref.ShardChecksums,
		ShardSizes:      existing.ref.ShardSizes,
	}, existing.meta.ContentType, existing.meta.UpdatedAt)
	if err != nil {
		return fmt.Errorf("restore previous layout: %w", err)
	}
	return nil
}

func (s *Store) rollbackBlobRef(ctx context.Context, hash string, mutation blobRefMutation) error {
	if mutation != blobRefCreated && mutation != blobRefIncreased {
		return nil
	}
	path, removed, err := s.meta.DecreaseBlobRef(ctx, hash)
	if err != nil {
		return fmt.Errorf("rollback blob ref: %w", mapStoreError(err))
	}
	if !removed {
		return nil
	}
	if err := s.engine.DeleteBlob(ctx, path, hash); err != nil {
		return fmt.Errorf("delete rollback blob: %w", err)
	}
	return nil
}

func (s *Store) cleanupCreatedBlob(ctx context.Context, blob engine.BlobInfo, createdBlob bool) error {
	if !createdBlob {
		return nil
	}
	if err := s.engine.DeleteBlob(ctx, blob.ShardDir, blob.Hash); err != nil {
		return fmt.Errorf("delete created blob: %w", err)
	}
	return nil
}

func (s *Store) releaseBlob(ctx context.Context, hash string) error {
	path, removed, err := s.meta.DecreaseBlobRef(ctx, hash)
	if err != nil {
		return mapStoreError(err)
	}
	if !removed {
		return nil
	}
	if err := s.engine.DeleteBlob(ctx, path, hash); err != nil {
		return fmt.Errorf("delete released blob: %w", err)
	}
	return nil
}

func objectMetaFromInfo(info engine.ObjectInfo) model.ObjectMeta {
	return model.ObjectMeta{
		Bucket:          info.Bucket,
		Key:             info.Key,
		Hash:            info.Hash,
		ETag:            info.ETag,
		Size:            info.Size,
		ContentType:     info.ContentType,
		UpdatedAt:       info.UpdatedAt,
		State:           model.ObjectStateCommitted,
		ShardPlacements: info.ShardPlacements,
		ShardChecksums:  info.ShardChecksums,
		ShardSizes:      info.ShardSizes,
	}
}
