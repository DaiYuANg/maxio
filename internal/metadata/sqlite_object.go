package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/model"
)

const sqliteObjectColumns = `bucket, object_key, hash, etag, size, content_type, cache_control, content_disposition,
content_encoding, content_language, user_metadata, updated_at, state, write_intent_id, write_intent_stage,
write_intent_started_at, write_intent_updated_at, shard_placements, shard_checksums, shard_sizes`

const sqliteObjectUpsertSQL = `INSERT INTO metadata_objects (
	bucket, object_key, hash, etag, size, content_type, cache_control, content_disposition,
	content_encoding, content_language, user_metadata, updated_at, state, write_intent_id,
	write_intent_stage, write_intent_started_at, write_intent_updated_at, shard_placements,
	shard_checksums, shard_sizes
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(bucket, object_key) DO UPDATE SET
	hash = excluded.hash,
	etag = excluded.etag,
	size = excluded.size,
	content_type = excluded.content_type,
	cache_control = excluded.cache_control,
	content_disposition = excluded.content_disposition,
	content_encoding = excluded.content_encoding,
	content_language = excluded.content_language,
	user_metadata = excluded.user_metadata,
	updated_at = excluded.updated_at,
	state = excluded.state,
	write_intent_id = excluded.write_intent_id,
	write_intent_stage = excluded.write_intent_stage,
	write_intent_started_at = excluded.write_intent_started_at,
	write_intent_updated_at = excluded.write_intent_updated_at,
	shard_placements = excluded.shard_placements,
	shard_checksums = excluded.shard_checksums,
	shard_sizes = excluded.shard_sizes`

func (s *SQLiteMetadata) ListObjectMetas(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)
	if bucket == "" {
		return nil, ErrBadRequest
	}
	if err := s.ensureBucket(ctx, bucket); err != nil {
		return nil, err
	}

	return s.queryObjectMetas(
		ctx,
		`SELECT `+sqliteObjectColumns+`
		   FROM metadata_objects
		  WHERE bucket = ? AND state = ? AND (? = '' OR object_key LIKE ?)
		  ORDER BY object_key ASC`,
		bucket,
		model.ObjectStateCommitted,
		prefix,
		prefixPattern(prefix),
	)
}

func (s *SQLiteMetadata) ListStagedObjectMetas(ctx context.Context, bucket, prefix string) ([]model.ObjectMeta, error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)
	if bucket != "" {
		if err := s.ensureBucket(ctx, bucket); err != nil {
			return nil, err
		}
	}

	return s.queryObjectMetas(
		ctx,
		`SELECT `+sqliteObjectColumns+`
		   FROM metadata_objects
		  WHERE state = ? AND (? = '' OR bucket = ?) AND (? = '' OR object_key LIKE ?)
		  ORDER BY bucket ASC, object_key ASC`,
		model.ObjectStatePending,
		bucket,
		bucket,
		prefix,
		prefixPattern(prefix),
	)
}

func (s *SQLiteMetadata) GetObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
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

func (s *SQLiteMetadata) StageObjectMeta(ctx context.Context, meta model.ObjectMeta) error {
	return s.writeObjectMeta(ctx, meta, model.ObjectStatePending, "stage")
}

func (s *SQLiteMetadata) UpsertObjectMeta(ctx context.Context, meta model.ObjectMeta) error {
	return s.writeObjectMeta(ctx, meta, model.ObjectStateCommitted, "upsert")
}

func (s *SQLiteMetadata) DeleteStagedObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	return s.deleteObjectMeta(ctx, bucket, key, model.ObjectStatePending, "staged")
}

func (s *SQLiteMetadata) DeleteObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	return s.deleteObjectMeta(ctx, bucket, key, model.ObjectStateCommitted, "committed")
}

func (s *SQLiteMetadata) queryObjectMetas(ctx context.Context, query string, args ...any) ([]model.ObjectMeta, error) {
	rows, err := s.queryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query object metas: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
			s.logger.Error("close sqlite rows", "rows", "object metas", "error", closeErr)
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

func (s *SQLiteMetadata) getObjectMeta(ctx context.Context, bucket, key, state string) (model.ObjectMeta, bool, error) {
	row := s.queryRowContext(ctx,
		`SELECT `+sqliteObjectColumns+`
		   FROM metadata_objects
		  WHERE bucket = ? AND object_key = ? AND state = ?
		  LIMIT 1`,
		bucket,
		key,
		state,
	)
	meta, err := scanObjectMeta(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return model.ObjectMeta{}, false, nil
		}
		return model.ObjectMeta{}, false, fmt.Errorf("query object meta: %w", err)
	}
	return meta, true, nil
}

func (s *SQLiteMetadata) writeObjectMeta(ctx context.Context, meta model.ObjectMeta, state, op string) error {
	meta, err := prepareObjectMeta(meta, state)
	if err != nil {
		return err
	}
	if err := s.ensureBucket(ctx, meta.Bucket); err != nil {
		return err
	}

	intentID, intentStage, intentStartedAt, intentUpdatedAt := extractWriteIntentValues(meta.WriteIntent)
	if _, err := s.execContext(
		ensureContext(ctx),
		sqliteObjectUpsertSQL,
		meta.Bucket,
		meta.Key,
		meta.Hash,
		meta.ETag,
		meta.Size,
		meta.ContentType,
		meta.CacheControl,
		meta.ContentDisposition,
		meta.ContentEncoding,
		meta.ContentLanguage,
		marshalUserMetadata(meta.UserMetadata),
		meta.UpdatedAt.UnixNano(),
		meta.State,
		intentID,
		intentStage,
		intentStartedAt,
		intentUpdatedAt,
		marshalShardPlacements(meta.ShardPlacements),
		marshalStrings(meta.ShardChecksums),
		marshalInt64s(meta.ShardSizes),
	); err != nil {
		return fmt.Errorf("%s object meta: %w", op, err)
	}
	return nil
}

func (s *SQLiteMetadata) deleteObjectMeta(
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

func (s *SQLiteMetadata) deleteObjectMetaRow(ctx context.Context, bucket, key, state string) (bool, error) {
	result, err := s.execContext(
		ensureContext(ctx),
		`DELETE FROM metadata_objects
		  WHERE bucket = ? AND object_key = ? AND state = ?`,
		bucket,
		key,
		state,
	)
	if err != nil {
		return false, fmt.Errorf("delete object meta: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete object meta rows: %w", err)
	}
	return affected > 0, nil
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
