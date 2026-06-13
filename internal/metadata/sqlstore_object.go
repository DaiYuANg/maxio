package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/arcgolabs/dbx/querydsl"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) ListObjectMetas(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)
	if bucket == "" {
		return nil, ErrBadRequest
	}
	if err := s.ensureBucket(ctx, bucket); err != nil {
		return nil, err
	}

	query := querydsl.SelectFrom(metadataObjects.table, metadataObjects.selectItems()...).
		Where(objectMetaFilter(bucket, prefix, model.ObjectStateCommitted)).
		OrderBy(metadataObjects.key.Asc())
	return s.queryObjectMetas(ctx, query)
}

func (s *SQLMetadata) ListStagedObjectMetas(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)
	if bucket != "" {
		if err := s.ensureBucket(ctx, bucket); err != nil {
			return nil, err
		}
	}

	query := querydsl.SelectFrom(metadataObjects.table, metadataObjects.selectItems()...).
		Where(objectMetaFilter(bucket, prefix, model.ObjectStatePending)).
		OrderBy(metadataObjects.bucket.Asc(), metadataObjects.key.Asc())
	return s.queryObjectMetas(ctx, query)
}

func (s *SQLMetadata) GetObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectMeta{}, false, ErrBadRequest
	}

	meta, found, err := s.getObjectMeta(ctx, bucket, key, model.ObjectStateCommitted)
	if err != nil {
		return model.ObjectMeta{}, false, fmt.Errorf("get object meta: %w", err)
	}
	return meta, found, nil
}

func (s *SQLMetadata) StageObjectMeta(ctx context.Context, meta model.ObjectMeta) error {
	return s.writeObjectMeta(ctx, meta, model.ObjectStatePending, "stage")
}

func (s *SQLMetadata) UpsertObjectMeta(ctx context.Context, meta model.ObjectMeta) error {
	return s.writeObjectMeta(ctx, meta, model.ObjectStateCommitted, "upsert")
}

func (s *SQLMetadata) DeleteStagedObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	return s.deleteObjectMeta(ctx, bucket, key, model.ObjectStatePending, "staged")
}

func (s *SQLMetadata) DeleteObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	return s.deleteObjectMeta(ctx, bucket, key, model.ObjectStateCommitted, "committed")
}

func (s *SQLMetadata) queryObjectMetas(ctx context.Context, query querydsl.Builder) ([]model.ObjectMeta, error) {
	rows, err := s.queryBuilderContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query object metas: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
			s.logger.Error("close sql metadata rows", "rows", "object metas", "error", closeErr)
		}
	}()

	metas := make([]model.ObjectMeta, 0)
	for rows.Next() {
		meta, err := scanObjectMeta(rows)
		if err != nil {
			return nil, err
		}
		metas = append(metas, meta)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate object metas: %w", err)
	}
	return metas, nil
}

func (s *SQLMetadata) getObjectMeta(ctx context.Context, bucket, key, state string) (model.ObjectMeta, bool, error) {
	query := querydsl.SelectFrom(metadataObjects.table, metadataObjects.selectItems()...).
		Where(querydsl.And(metadataObjects.bucket.Eq(bucket), metadataObjects.key.Eq(key), metadataObjects.state.Eq(state))).
		Limit(1)
	row, err := s.queryRowBuilderContext(ctx, query)
	if err != nil {
		return model.ObjectMeta{}, false, fmt.Errorf("query object meta: %w", err)
	}
	meta, err := scanObjectMeta(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return model.ObjectMeta{}, false, nil
		}
		return model.ObjectMeta{}, false, fmt.Errorf("query object meta: %w", err)
	}
	return meta, true, nil
}

func (s *SQLMetadata) writeObjectMeta(ctx context.Context, meta model.ObjectMeta, state, op string) error {
	meta, err := prepareObjectMeta(meta, state)
	if err != nil {
		return err
	}
	if err := s.ensureBucket(ctx, meta.Bucket); err != nil {
		return err
	}

	intentID, intentStage, intentStartedAt, intentUpdatedAt := extractWriteIntentValues(meta.WriteIntent)
	query := querydsl.InsertInto(metadataObjects.table).
		Values(
			metadataObjects.bucket.Set(meta.Bucket),
			metadataObjects.key.Set(meta.Key),
			metadataObjects.hash.Set(meta.Hash),
			metadataObjects.etag.Set(meta.ETag),
			metadataObjects.size.Set(meta.Size),
			metadataObjects.contentType.Set(meta.ContentType),
			metadataObjects.cacheControl.Set(meta.CacheControl),
			metadataObjects.contentDisposition.Set(meta.ContentDisposition),
			metadataObjects.contentEncoding.Set(meta.ContentEncoding),
			metadataObjects.contentLanguage.Set(meta.ContentLanguage),
			metadataObjects.userMetadata.Set(marshalUserMetadata(meta.UserMetadata)),
			metadataObjects.updatedAt.Set(meta.UpdatedAt.UnixNano()),
			metadataObjects.state.Set(meta.State),
			metadataObjects.writeIntentID.Set(intentID),
			metadataObjects.writeIntentStage.Set(intentStage),
			metadataObjects.writeIntentStartedAt.Set(intentStartedAt),
			metadataObjects.writeIntentUpdatedAt.Set(intentUpdatedAt),
			metadataObjects.shardPlacements.Set(marshalShardPlacements(meta.ShardPlacements)),
			metadataObjects.shardChecksums.Set(marshalStrings(meta.ShardChecksums)),
			metadataObjects.shardSizes.Set(marshalInt64s(meta.ShardSizes)),
		).
		OnConflict(metadataObjects.bucket, metadataObjects.key).
		DoUpdateSet(
			metadataObjects.hash.SetExcluded(),
			metadataObjects.etag.SetExcluded(),
			metadataObjects.size.SetExcluded(),
			metadataObjects.contentType.SetExcluded(),
			metadataObjects.cacheControl.SetExcluded(),
			metadataObjects.contentDisposition.SetExcluded(),
			metadataObjects.contentEncoding.SetExcluded(),
			metadataObjects.contentLanguage.SetExcluded(),
			metadataObjects.userMetadata.SetExcluded(),
			metadataObjects.updatedAt.SetExcluded(),
			metadataObjects.state.SetExcluded(),
			metadataObjects.writeIntentID.SetExcluded(),
			metadataObjects.writeIntentStage.SetExcluded(),
			metadataObjects.writeIntentStartedAt.SetExcluded(),
			metadataObjects.writeIntentUpdatedAt.SetExcluded(),
			metadataObjects.shardPlacements.SetExcluded(),
			metadataObjects.shardChecksums.SetExcluded(),
			metadataObjects.shardSizes.SetExcluded(),
		)
	if _, err := s.execBuilderContext(ensureContext(ctx), query); err != nil {
		return fmt.Errorf("%s object meta: %w", op, err)
	}
	return nil
}

func (s *SQLMetadata) deleteObjectMeta(
	ctx context.Context,
	bucket string,
	key string,
	state string,
	label string,
) (model.ObjectMeta, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectMeta{}, false, ErrBadRequest
	}

	meta, found, err := s.getObjectMeta(ctx, bucket, key, state)
	if err != nil {
		return model.ObjectMeta{}, false, fmt.Errorf("get %s object meta: %w", label, err)
	}
	if !found {
		return model.ObjectMeta{}, false, nil
	}
	deleted, err := s.deleteObjectMetaRow(ctx, bucket, key, state)
	if err != nil || !deleted {
		return model.ObjectMeta{}, false, err
	}
	return meta, true, nil
}

func (s *SQLMetadata) deleteObjectMetaRow(ctx context.Context, bucket, key, state string) (bool, error) {
	query := querydsl.DeleteFrom(metadataObjects.table).
		Where(querydsl.And(metadataObjects.bucket.Eq(bucket), metadataObjects.key.Eq(key), metadataObjects.state.Eq(state)))
	result, err := s.execBuilderContext(ensureContext(ctx), query)
	if err != nil {
		return false, fmt.Errorf("delete object meta: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete object meta rows: %w", err)
	}
	return affected > 0, nil
}

func objectMetaFilter(bucket, prefix, state string) querydsl.Predicate {
	predicates := []querydsl.Predicate{metadataObjects.state.Eq(state)}
	if bucket != "" {
		predicates = append(predicates, metadataObjects.bucket.Eq(bucket))
	}
	if prefix != "" {
		predicates = append(predicates, querydsl.Like(metadataObjects.key, prefixPattern(prefix)))
	}
	return querydsl.And(predicates...)
}

func prepareObjectMeta(meta model.ObjectMeta, state string) (model.ObjectMeta, error) {
	meta.Bucket = strings.TrimSpace(meta.Bucket)
	meta.Key = strings.TrimSpace(meta.Key)
	meta.Hash = strings.TrimSpace(meta.Hash)
	if meta.Bucket == "" || meta.Key == "" || meta.Hash == "" {
		return model.ObjectMeta{}, ErrBadRequest
	}
	meta.State = state
	meta.UpdatedAt = time.Now().UTC()
	return meta, nil
}

func prefixPattern(prefix string) string {
	if prefix == "" {
		return ""
	}
	return prefix + "%"
}

func errorsIsNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
