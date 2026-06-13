package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

const sqlStoreObjectRecordColumns = `bucket, object_key, current_version_id, deleted, created_at, updated_at`

const sqlStoreObjectVersionColumns = `bucket, object_key, version_id, digest, etag, size, content_type, cache_control,
content_disposition, content_encoding, content_language, user_metadata, upstream_id, upstream_bucket, upstream_key,
delete_marker, created_at, updated_at`

const sqlStoreDigestRefColumns = `digest, size, ref_count, upstream_id, upstream_bucket, upstream_key, created_at, updated_at`

const sqlStoreObjectRecordUpsertSQL = `INSERT INTO metadata_object_records (
	bucket, object_key, current_version_id, deleted, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(bucket, object_key) DO UPDATE SET
	current_version_id = excluded.current_version_id,
	deleted = excluded.deleted,
	updated_at = excluded.updated_at`

const sqlStoreObjectVersionUpsertSQL = `INSERT INTO metadata_object_versions (
	bucket, object_key, version_id, digest, etag, size, content_type, cache_control,
	content_disposition, content_encoding, content_language, user_metadata, upstream_id,
	upstream_bucket, upstream_key, delete_marker, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(bucket, object_key, version_id) DO UPDATE SET
	digest = excluded.digest,
	etag = excluded.etag,
	size = excluded.size,
	content_type = excluded.content_type,
	cache_control = excluded.cache_control,
	content_disposition = excluded.content_disposition,
	content_encoding = excluded.content_encoding,
	content_language = excluded.content_language,
	user_metadata = excluded.user_metadata,
	upstream_id = excluded.upstream_id,
	upstream_bucket = excluded.upstream_bucket,
	upstream_key = excluded.upstream_key,
	delete_marker = excluded.delete_marker,
	updated_at = excluded.updated_at`

const sqlStoreDigestRefUpsertSQL = `INSERT INTO metadata_digest_refs (
	digest, size, ref_count, upstream_id, upstream_bucket, upstream_key, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(digest) DO UPDATE SET
	size = excluded.size,
	ref_count = excluded.ref_count,
	upstream_id = excluded.upstream_id,
	upstream_bucket = excluded.upstream_bucket,
	upstream_key = excluded.upstream_key,
	updated_at = excluded.updated_at`

func (s *SQLMetadata) UpsertObjectRecord(ctx context.Context, record model.ObjectRecord) (model.ObjectRecord, error) {
	record, err := prepareDBObjectRecord(record)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	if ensureErr := s.ensureBucket(ctx, record.Bucket); ensureErr != nil {
		return model.ObjectRecord{}, ensureErr
	}
	if _, execErr := s.execContext(
		ctx,
		sqlStoreObjectRecordUpsertSQL,
		record.Bucket,
		record.Key,
		record.CurrentVersionID,
		boolToInt(record.Deleted),
		record.CreatedAt.UnixNano(),
		record.UpdatedAt.UnixNano(),
	); execErr != nil {
		return model.ObjectRecord{}, fmt.Errorf("upsert object record: %w", execErr)
	}
	stored, found, err := s.GetObjectRecord(ctx, record.Bucket, record.Key)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	if !found {
		return model.ObjectRecord{}, ErrObjectNotFound
	}
	return stored, nil
}

func (s *SQLMetadata) GetObjectRecord(ctx context.Context, bucket, key string) (model.ObjectRecord, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectRecord{}, false, ErrBadRequest
	}

	row := s.queryRowContext(
		ctx,
		`SELECT `+sqlStoreObjectRecordColumns+`
		   FROM metadata_object_records
		  WHERE bucket = ? AND object_key = ?
		  LIMIT 1`,
		bucket,
		key,
	)
	record, err := scanObjectRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ObjectRecord{}, false, nil
	}
	if err != nil {
		return model.ObjectRecord{}, false, fmt.Errorf("get object record: %w", err)
	}
	return record, true, nil
}

func (s *SQLMetadata) DeleteObjectRecord(ctx context.Context, bucket, key string) (bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return false, ErrBadRequest
	}
	result, err := s.execContext(
		ctx,
		`DELETE FROM metadata_object_records WHERE bucket = ? AND object_key = ?`,
		bucket,
		key,
	)
	if err != nil {
		return false, fmt.Errorf("delete object record: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete object record rows: %w", err)
	}
	return affected > 0, nil
}

func (s *SQLMetadata) UpsertObjectVersion(ctx context.Context, version model.ObjectVersion) (model.ObjectVersion, error) {
	version, err := prepareDBObjectVersion(version)
	if err != nil {
		return model.ObjectVersion{}, err
	}
	if ensureErr := s.ensureBucket(ctx, version.Bucket); ensureErr != nil {
		return model.ObjectVersion{}, ensureErr
	}
	if _, execErr := s.execContext(
		ctx,
		sqlStoreObjectVersionUpsertSQL,
		version.Bucket,
		version.Key,
		version.VersionID,
		version.Digest,
		version.ETag,
		version.Size,
		version.ContentType,
		version.CacheControl,
		version.ContentDisposition,
		version.ContentEncoding,
		version.ContentLanguage,
		marshalUserMetadata(version.UserMetadata),
		version.UpstreamID,
		version.UpstreamBucket,
		version.UpstreamKey,
		boolToInt(version.DeleteMarker),
		version.CreatedAt.UnixNano(),
		version.UpdatedAt.UnixNano(),
	); execErr != nil {
		return model.ObjectVersion{}, fmt.Errorf("upsert object version: %w", execErr)
	}
	stored, found, err := s.GetObjectVersion(ctx, version.Bucket, version.Key, version.VersionID)
	if err != nil {
		return model.ObjectVersion{}, err
	}
	if !found {
		return model.ObjectVersion{}, ErrObjectNotFound
	}
	return stored, nil
}

func (s *SQLMetadata) GetObjectVersion(ctx context.Context, bucket, key, versionID string) (model.ObjectVersion, bool, error) {
	bucket, key, versionID = trimObjectVersionKey(bucket, key, versionID)
	if bucket == "" || key == "" || versionID == "" {
		return model.ObjectVersion{}, false, ErrBadRequest
	}

	row := s.queryRowContext(
		ctx,
		`SELECT `+sqlStoreObjectVersionColumns+`
		   FROM metadata_object_versions
		  WHERE bucket = ? AND object_key = ? AND version_id = ?
		  LIMIT 1`,
		bucket,
		key,
		versionID,
	)
	version, err := scanObjectVersion(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ObjectVersion{}, false, nil
	}
	if err != nil {
		return model.ObjectVersion{}, false, fmt.Errorf("get object version: %w", err)
	}
	return version, true, nil
}

func (s *SQLMetadata) ListObjectVersions(ctx context.Context, bucket, key string) ([]model.ObjectVersion, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return nil, ErrBadRequest
	}

	rows, err := s.queryContext(
		ctx,
		`SELECT `+sqlStoreObjectVersionColumns+`
		   FROM metadata_object_versions
		  WHERE bucket = ? AND object_key = ?
		  ORDER BY created_at DESC, version_id DESC`,
		bucket,
		key,
	)
	if err != nil {
		return nil, fmt.Errorf("query object versions: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
			s.logger.Error("close sql metadata rows", "rows", "object versions", "error", closeErr)
		}
	}()

	versions := make([]model.ObjectVersion, 0)
	for rows.Next() {
		version, err := scanObjectVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate object versions: %w", err)
	}
	return versions, nil
}

func (s *SQLMetadata) DeleteObjectVersion(ctx context.Context, bucket, key, versionID string) (bool, error) {
	bucket, key, versionID = trimObjectVersionKey(bucket, key, versionID)
	if bucket == "" || key == "" || versionID == "" {
		return false, ErrBadRequest
	}
	result, err := s.execContext(
		ctx,
		`DELETE FROM metadata_object_versions
		  WHERE bucket = ? AND object_key = ? AND version_id = ?`,
		bucket,
		key,
		versionID,
	)
	if err != nil {
		return false, fmt.Errorf("delete object version: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete object version rows: %w", err)
	}
	return affected > 0, nil
}
