package store

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/model"
)

type existingObject struct {
	meta      model.ObjectMeta
	ref       metadata.BlobRef
	exists    bool
	refExists bool
}

func (s *Store) PutObject(ctx context.Context, bucket, key string, reader io.Reader, opts PutOptions) (model.ObjectMeta, error) {
	bucket, key, err := normalizeObjectLocation(bucket, key)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	opts = opts.normalized()
	if ensureErr := s.ensureBucketExists(ctx, bucket); ensureErr != nil {
		return model.ObjectMeta{}, ensureErr
	}

	existing, err := s.loadExistingObject(ctx, bucket, key)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	staged, err := stageObject(reader)
	if err != nil {
		return model.ObjectMeta{}, fmt.Errorf("%w: stage object input: %w", ErrEngineFailed, err)
	}
	defer closeStagedObject(staged)

	return s.putStagedObject(ctx, bucket, key, opts, staged, existing)
}

func (s *Store) putStagedObject(
	ctx context.Context,
	bucket string,
	key string,
	opts PutOptions,
	staged *stagedObject,
	existing existingObject,
) (model.ObjectMeta, error) {
	pending, err := s.stageObjectMeta(ctx, bucket, key, opts, staged)
	if err != nil {
		return model.ObjectMeta{}, err
	}

	blob, refExists, createdBlob, err := s.preparePutBlob(ctx, bucket, key, staged)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	return s.completePreparedPut(ctx, bucket, key, opts, staged, existing, pending, blob, refExists, createdBlob)
}

func (s *Store) completePreparedPut(
	ctx context.Context,
	bucket string,
	key string,
	opts PutOptions,
	staged *stagedObject,
	existing existingObject,
	pending model.ObjectMeta,
	blob engine.BlobInfo,
	refExists bool,
	createdBlob bool,
) (model.ObjectMeta, error) {
	pending, err := s.updatePendingWriteIntent(ctx, pending, model.WriteIntentStageBlobPrepared)
	if err != nil {
		return model.ObjectMeta{}, s.failPut(ctx, bucket, key, staged.hash, blob, blobRefUnchanged, createdBlob, existing, err)
	}
	info, err := s.linkPutLayout(ctx, bucket, key, opts.ContentType, blob, createdBlob)
	if err != nil {
		return model.ObjectMeta{}, err
	}
	pending, err = s.updatePendingWriteIntent(ctx, pending, model.WriteIntentStageLayoutLinked)
	if err != nil {
		return model.ObjectMeta{}, s.failPut(ctx, bucket, key, staged.hash, blob, blobRefUnchanged, createdBlob, existing, err)
	}

	mutation, err := s.retainBlob(ctx, staged.hash, blob, refExists, existing.exists && existing.meta.Hash == staged.hash)
	if err != nil {
		return model.ObjectMeta{}, s.failPut(ctx, bucket, key, staged.hash, blob, blobRefUnchanged, createdBlob, existing, err)
	}
	pending, err = s.updatePendingWriteIntent(ctx, pending, model.WriteIntentStageBlobRetained)
	if err != nil {
		return model.ObjectMeta{}, s.failPut(ctx, bucket, key, staged.hash, blob, mutation, createdBlob, existing, err)
	}

	meta := opts.apply(objectMetaFromInfo(info))
	meta.WriteIntent = advanceWriteIntent(pending.WriteIntent, model.WriteIntentStageCommitted, time.Now().UTC())
	if err := s.meta.UpsertObjectMeta(ctx, meta); err != nil {
		wrapped := fmt.Errorf("upsert object metadata: %w", mapStoreError(err))
		return model.ObjectMeta{}, s.failPut(ctx, bucket, key, staged.hash, blob, mutation, createdBlob, existing, wrapped)
	}

	if err := s.deleteStagedObjectMeta(ctx, bucket, key); err != nil {
		return model.ObjectMeta{}, err
	}

	if err := s.releaseReplacedBlob(ctx, existing, staged.hash); err != nil {
		return model.ObjectMeta{}, err
	}
	return meta, nil
}

func (s *Store) preparePutBlob(
	ctx context.Context,
	bucket string,
	key string,
	staged *stagedObject,
) (engine.BlobInfo, bool, bool, error) {
	blob, refExists, createdBlob, err := s.prepareBlob(ctx, key, staged)
	if err == nil {
		return blob, refExists, createdBlob, nil
	}
	if cleanupErr := s.deleteStagedObjectMeta(ctx, bucket, key); cleanupErr != nil {
		return engine.BlobInfo{}, false, false, fmt.Errorf("%w; cleanup staged metadata: %w", err, cleanupErr)
	}
	return engine.BlobInfo{}, false, false, err
}

func (s *Store) stageObjectMeta(
	ctx context.Context,
	bucket string,
	key string,
	opts PutOptions,
	staged *stagedObject,
) (model.ObjectMeta, error) {
	now := time.Now().UTC()
	meta := opts.apply(model.ObjectMeta{
		Bucket:    bucket,
		Key:       key,
		Hash:      staged.hash,
		ETag:      engine.ETagFromHash(staged.hash),
		Size:      staged.size,
		UpdatedAt: now,
		State:     model.ObjectStatePending,
	})
	meta.WriteIntent = newWriteIntent(bucket, key, staged.hash, now)
	if err := s.meta.StageObjectMeta(ctx, meta); err != nil {
		return model.ObjectMeta{}, fmt.Errorf("stage object metadata: %w", mapStoreError(err))
	}
	return meta, nil
}

func (s *Store) deleteStagedObjectMeta(ctx context.Context, bucket, key string) error {
	if _, _, err := s.meta.DeleteStagedObjectMeta(ctx, bucket, key); err != nil {
		return fmt.Errorf("delete staged object metadata: %w", mapStoreError(err))
	}
	return nil
}

func normalizeObjectLocation(bucket, key string) (string, string, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return "", "", ErrBadRequest
	}
	return bucket, key, nil
}

func (s *Store) ensureBucketExists(ctx context.Context, bucket string) error {
	exists, err := s.meta.BucketExists(ctx, bucket)
	if err != nil {
		return mapStoreError(err)
	}
	if !exists {
		return ErrBucketNotFound
	}
	return nil
}

func (s *Store) loadExistingObject(ctx context.Context, bucket, key string) (existingObject, error) {
	meta, exists, err := s.meta.GetObjectMeta(ctx, bucket, key)
	if err != nil {
		return existingObject{}, mapStoreError(err)
	}
	if !exists {
		return existingObject{}, nil
	}
	ref, refExists, err := s.meta.GetBlobRef(ctx, meta.Hash)
	if err != nil {
		return existingObject{}, mapStoreError(err)
	}
	return existingObject{
		meta:      meta,
		ref:       ref,
		exists:    true,
		refExists: refExists,
	}, nil
}

func (s *Store) linkPutLayout(
	ctx context.Context,
	bucket string,
	key string,
	contentType string,
	blob engine.BlobInfo,
	createdBlob bool,
) (engine.ObjectInfo, error) {
	info, err := s.engine.LinkObject(ctx, bucket, key, blob, contentType, time.Now().UTC())
	if err == nil {
		return info, nil
	}
	if createdBlob {
		if cleanupErr := s.engine.DeleteBlob(ctx, blob.ShardDir, blob.Hash); cleanupErr != nil {
			return engine.ObjectInfo{}, fmt.Errorf("%w: %w; cleanup new blob: %w", ErrEngineFailed, err, cleanupErr)
		}
	}
	return engine.ObjectInfo{}, fmt.Errorf("%w: %w", ErrEngineFailed, err)
}

func (s *Store) releaseReplacedBlob(ctx context.Context, existing existingObject, newHash string) error {
	if !existing.exists || existing.meta.Hash == newHash {
		return nil
	}
	if err := s.releaseBlob(ctx, existing.meta.Hash); err != nil {
		return fmt.Errorf("release old object blob: %w", err)
	}
	return nil
}

func (s *Store) failPut(
	ctx context.Context,
	bucket string,
	key string,
	hash string,
	blob engine.BlobInfo,
	mutation blobRefMutation,
	createdBlob bool,
	existing existingObject,
	cause error,
) error {
	if rollbackErr := s.rollbackPut(ctx, bucket, key, hash, blob, mutation, createdBlob, existing); rollbackErr != nil {
		return fmt.Errorf("%w; rollback: %w", cause, rollbackErr)
	}
	return cause
}
