package metadata

import (
	"context"
	"errors"
	"time"

	raftx "github.com/lyonbrown4d/maxio/internal/raft"
	"github.com/lyonbrown4d/maxio/model"
)

type RaftMetadata struct {
	runtime *raftx.Runtime
}

func NewRaftMetadata(runtime *raftx.Runtime) (*RaftMetadata, error) {
	if runtime == nil {
		return nil, errors.New("raft runtime is nil")
	}
	return &RaftMetadata{runtime: runtime}, nil
}

func (r *RaftMetadata) ListBuckets(ctx context.Context) ([]model.Bucket, error) {
	result, err := r.runtime.ReadMetadata(ctx, raftx.MetadataQuery{Type: raftx.MetadataQueryListBuckets})
	if err != nil {
		return nil, mapRaftError(err)
	}
	return result.Buckets, nil
}

func (r *RaftMetadata) BucketExists(ctx context.Context, bucket string) (bool, error) {
	result, err := r.runtime.ReadMetadata(ctx, raftx.MetadataQuery{
		Type:   raftx.MetadataQueryBucketExists,
		Bucket: bucket,
	})
	if err != nil {
		return false, mapRaftError(err)
	}
	return result.BucketExists, nil
}

func (r *RaftMetadata) CreateBucket(ctx context.Context, bucket string) error {
	_, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type: raftx.MetadataCommandCreateBucket,
		BucketMeta: model.Bucket{
			Name:      bucket,
			CreatedAt: time.Now(),
		},
	})
	return mapRaftError(err)
}

func (r *RaftMetadata) DeleteBucket(ctx context.Context, bucket string) error {
	_, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type:   raftx.MetadataCommandDeleteBucket,
		Bucket: bucket,
	})
	return mapRaftError(err)
}

func (r *RaftMetadata) ListObjectMetas(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	result, err := r.runtime.ReadMetadata(ctx, raftx.MetadataQuery{
		Type:   raftx.MetadataQueryListObjectMetas,
		Bucket: bucket,
		Prefix: prefix,
	})
	if err != nil {
		return nil, mapRaftError(err)
	}
	return result.Objects, nil
}

func (r *RaftMetadata) ListStagedObjectMetas(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	result, err := r.runtime.ReadMetadata(ctx, raftx.MetadataQuery{
		Type:   raftx.MetadataQueryListStagedObjectMetas,
		Bucket: bucket,
		Prefix: prefix,
	})
	if err != nil {
		return nil, mapRaftError(err)
	}
	return result.Objects, nil
}

func (r *RaftMetadata) ListBlobRefs(ctx context.Context) ([]BlobRef, error) {
	result, err := r.runtime.ReadMetadata(ctx, raftx.MetadataQuery{Type: raftx.MetadataQueryListBlobRefs})
	if err != nil {
		return nil, mapRaftError(err)
	}
	refs := make([]BlobRef, 0, len(result.Blobs))
	for index := range result.Blobs {
		refs = append(refs, fromRaftBlobRef(result.Blobs[index]))
	}
	return refs, nil
}

func (r *RaftMetadata) GetObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	result, err := r.runtime.ReadMetadata(ctx, raftx.MetadataQuery{
		Type:   raftx.MetadataQueryGetObjectMeta,
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		return model.ObjectMeta{}, false, mapRaftError(err)
	}
	return result.Object, result.ObjectExists, nil
}

func (r *RaftMetadata) StageObjectMeta(ctx context.Context, meta model.ObjectMeta) error {
	meta.State = model.ObjectStatePending
	_, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type: raftx.MetadataCommandStageObjectMeta,
		Meta: meta,
	})
	return mapRaftError(err)
}

func (r *RaftMetadata) UpsertObjectMeta(ctx context.Context, meta model.ObjectMeta) error {
	meta.State = model.ObjectStateCommitted
	_, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type: raftx.MetadataCommandUpsertObjectMeta,
		Meta: meta,
	})
	return mapRaftError(err)
}

func (r *RaftMetadata) DeleteStagedObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	result, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type:   raftx.MetadataCommandDeleteStagedObjectMeta,
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		return model.ObjectMeta{}, false, mapRaftError(err)
	}
	return result.Object, result.ObjectExists, nil
}

func (r *RaftMetadata) DeleteObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	result, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type:   raftx.MetadataCommandDeleteObjectMeta,
		Bucket: bucket,
		Key:    key,
	})
	if err != nil {
		return model.ObjectMeta{}, false, mapRaftError(err)
	}
	return result.Object, result.ObjectExists, nil
}

func (r *RaftMetadata) GetBlobRef(ctx context.Context, hash string) (BlobRef, bool, error) {
	result, err := r.runtime.ReadMetadata(ctx, raftx.MetadataQuery{
		Type: raftx.MetadataQueryGetBlobRef,
		Hash: hash,
	})
	if err != nil {
		return BlobRef{}, false, mapRaftError(err)
	}
	return fromRaftBlobRef(result.Blob), result.BlobExists, nil
}

func (r *RaftMetadata) CreateBlobRef(
	ctx context.Context,
	hash string,
	path string,
	size int64,
	placements []model.ShardPlacement,
	checksums []string,
	shardSizes ...[]int64,
) error {
	_, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type:            raftx.MetadataCommandCreateBlobRef,
		Hash:            hash,
		Path:            path,
		Size:            size,
		ShardPlacements: placements,
		ShardChecksums:  checksums,
		ShardSizes:      firstShardSizes(shardSizes),
	})
	return mapRaftError(err)
}

func (r *RaftMetadata) UpdateBlobRefPlacements(
	ctx context.Context,
	hash string,
	placements []model.ShardPlacement,
) error {
	_, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type:            raftx.MetadataCommandUpdateBlobRefPlacements,
		Hash:            hash,
		ShardPlacements: placements,
	})
	return mapRaftError(err)
}

func (r *RaftMetadata) IncreaseBlobRef(ctx context.Context, hash string) error {
	_, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type: raftx.MetadataCommandIncreaseBlobRef,
		Hash: hash,
	})
	return mapRaftError(err)
}

func (r *RaftMetadata) DecreaseBlobRef(ctx context.Context, hash string) (string, bool, error) {
	result, err := r.runtime.ProposeMetadata(ctx, raftx.MetadataCommand{
		Type: raftx.MetadataCommandDecreaseBlobRef,
		Hash: hash,
	})
	if err != nil {
		return "", false, mapRaftError(err)
	}
	return result.BlobPath, result.BlobRemoved, nil
}

func mapRaftError(err error) error {
	if err == nil {
		return nil
	}
	switch raftx.MetadataErrorCode(err) {
	case raftx.MetadataErrorBadRequest:
		return ErrBadRequest
	case raftx.MetadataErrorBucketExists:
		return ErrBucketExists
	case raftx.MetadataErrorBucketNotFound:
		return ErrBucketNotFound
	case raftx.MetadataErrorObjectNotFound:
		return ErrObjectNotFound
	default:
		return err
	}
}

func fromRaftBlobRef(ref raftx.MetadataBlobRef) BlobRef {
	return BlobRef{
		Hash:            ref.Hash,
		Path:            ref.Path,
		ShardPlacements: ref.ShardPlacements,
		ShardChecksums:  ref.ShardChecksums,
		ShardSizes:      ref.ShardSizes,
		RefCount:        ref.RefCount,
		Size:            ref.Size,
	}
}

func firstShardSizes(input [][]int64) []int64 {
	if len(input) == 0 {
		return nil
	}
	return input[0]
}
