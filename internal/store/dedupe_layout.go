package store

import (
	"context"
	"fmt"
	"reflect"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/model"
)

func (s *Store) canonicalizeDedupeLayouts(
	ctx context.Context,
	opts DedupeOptions,
	objects []dedupeObject,
	refs map[string]metadata.BlobRef,
	result *DedupeResult,
) error {
	for index := range objects {
		object := objects[index]
		ref, ok := refs[object.meta.Hash]
		if !ok || !object.hasInfo || !dedupeLayoutMismatch(object.info, ref) {
			continue
		}
		recordDedupeLayoutMismatch(object, ref, result)
		if opts.DryRun || result.reachedLimit(opts) {
			continue
		}
		if err := s.canonicalizeDedupeLayout(ctx, object.meta, ref); err != nil {
			return err
		}
		result.LayoutsCanonicalized++
		result.Fixes++
	}
	return nil
}

func recordDedupeLayoutMismatch(object dedupeObject, ref metadata.BlobRef, result *DedupeResult) {
	result.LayoutsMismatched++
	result.addIssue(DedupeIssue{
		Kind:          DedupeIssueLayoutMismatch,
		Hash:          object.meta.Hash,
		Bucket:        object.meta.Bucket,
		Key:           object.meta.Key,
		Path:          object.info.ShardDir,
		CanonicalPath: ref.Path,
		Size:          object.meta.Size,
	})
}

func (s *Store) canonicalizeDedupeLayout(ctx context.Context, meta model.ObjectMeta, ref metadata.BlobRef) error {
	info, err := s.engine.LinkObject(ctx, meta.Bucket, meta.Key, engine.BlobInfo{
		Hash:            ref.Hash,
		ETag:            engine.ETagFromHash(ref.Hash),
		Size:            ref.Size,
		ShardDir:        ref.Path,
		ShardPlacements: ref.ShardPlacements,
		ShardChecksums:  ref.ShardChecksums,
		ShardSizes:      ref.ShardSizes,
	}, meta.ContentType, meta.UpdatedAt)
	if err != nil {
		return fmt.Errorf("canonicalize dedupe layout: %w", mapStoreError(err))
	}
	meta.Hash = info.Hash
	meta.ETag = info.ETag
	meta.Size = info.Size
	meta.State = model.ObjectStateCommitted
	meta.ShardPlacements = info.ShardPlacements
	meta.ShardChecksums = info.ShardChecksums
	meta.ShardSizes = info.ShardSizes
	if err := s.meta.UpsertObjectMeta(ctx, meta); err != nil {
		return fmt.Errorf("update canonical dedupe object metadata: %w", mapStoreError(err))
	}
	return nil
}

func dedupeLayoutMismatch(info engine.ObjectInfo, ref metadata.BlobRef) bool {
	return info.ShardDir != ref.Path ||
		!reflect.DeepEqual(info.ShardPlacements, ref.ShardPlacements) ||
		!reflect.DeepEqual(info.ShardChecksums, ref.ShardChecksums) ||
		!reflect.DeepEqual(info.ShardSizes, ref.ShardSizes)
}
