package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/arcgolabs/dbx/querydsl"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) UpsertObjectRecord(ctx context.Context, record model.ObjectRecord) (model.ObjectRecord, error) {
	record, err := prepareDBObjectRecord(record)
	if err != nil {
		return model.ObjectRecord{}, err
	}
	if ensureErr := s.ensureBucket(ctx, record.Bucket); ensureErr != nil {
		return model.ObjectRecord{}, ensureErr
	}
	query := querydsl.InsertInto(metadataObjectRecords.table).
		Values(
			metadataObjectRecords.bucket.Set(record.Bucket),
			metadataObjectRecords.key.Set(record.Key),
			metadataObjectRecords.currentVersionID.Set(record.CurrentVersionID),
			metadataObjectRecords.deleted.Set(boolToInt(record.Deleted)),
			metadataObjectRecords.createdAt.Set(record.CreatedAt.UnixNano()),
			metadataObjectRecords.updatedAt.Set(record.UpdatedAt.UnixNano()),
		).
		OnConflict(metadataObjectRecords.bucket, metadataObjectRecords.key).
		DoUpdateSet(
			metadataObjectRecords.currentVersionID.SetExcluded(),
			metadataObjectRecords.deleted.SetExcluded(),
			metadataObjectRecords.updatedAt.SetExcluded(),
		)
	if _, execErr := s.execBuilderContext(ctx, query); execErr != nil {
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

	query := querydsl.SelectFrom(metadataObjectRecords.table, metadataObjectRecords.selectItems()...).
		Where(querydsl.And(metadataObjectRecords.bucket.Eq(bucket), metadataObjectRecords.key.Eq(key))).
		Limit(1)
	row, queryErr := s.queryRowBuilderContext(ctx, query)
	if queryErr != nil {
		return model.ObjectRecord{}, false, fmt.Errorf("get object record: %w", queryErr)
	}
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
	query := querydsl.DeleteFrom(metadataObjectRecords.table).
		Where(querydsl.And(metadataObjectRecords.bucket.Eq(bucket), metadataObjectRecords.key.Eq(key)))
	result, err := s.execBuilderContext(ctx, query)
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
	query := querydsl.InsertInto(metadataObjectVersions.table).
		Values(
			metadataObjectVersions.bucket.Set(version.Bucket),
			metadataObjectVersions.key.Set(version.Key),
			metadataObjectVersions.versionID.Set(version.VersionID),
			metadataObjectVersions.digest.Set(version.Digest),
			metadataObjectVersions.etag.Set(version.ETag),
			metadataObjectVersions.size.Set(version.Size),
			metadataObjectVersions.contentType.Set(version.ContentType),
			metadataObjectVersions.cacheControl.Set(version.CacheControl),
			metadataObjectVersions.contentDisposition.Set(version.ContentDisposition),
			metadataObjectVersions.contentEncoding.Set(version.ContentEncoding),
			metadataObjectVersions.contentLanguage.Set(version.ContentLanguage),
			metadataObjectVersions.userMetadata.Set(marshalUserMetadata(version.UserMetadata)),
			metadataObjectVersions.upstreamID.Set(version.UpstreamID),
			metadataObjectVersions.upstreamBucket.Set(version.UpstreamBucket),
			metadataObjectVersions.upstreamKey.Set(version.UpstreamKey),
			metadataObjectVersions.deleteMarker.Set(boolToInt(version.DeleteMarker)),
			metadataObjectVersions.createdAt.Set(version.CreatedAt.UnixNano()),
			metadataObjectVersions.updatedAt.Set(version.UpdatedAt.UnixNano()),
		).
		OnConflict(metadataObjectVersions.bucket, metadataObjectVersions.key, metadataObjectVersions.versionID).
		DoUpdateSet(
			metadataObjectVersions.digest.SetExcluded(),
			metadataObjectVersions.etag.SetExcluded(),
			metadataObjectVersions.size.SetExcluded(),
			metadataObjectVersions.contentType.SetExcluded(),
			metadataObjectVersions.cacheControl.SetExcluded(),
			metadataObjectVersions.contentDisposition.SetExcluded(),
			metadataObjectVersions.contentEncoding.SetExcluded(),
			metadataObjectVersions.contentLanguage.SetExcluded(),
			metadataObjectVersions.userMetadata.SetExcluded(),
			metadataObjectVersions.upstreamID.SetExcluded(),
			metadataObjectVersions.upstreamBucket.SetExcluded(),
			metadataObjectVersions.upstreamKey.SetExcluded(),
			metadataObjectVersions.deleteMarker.SetExcluded(),
			metadataObjectVersions.updatedAt.SetExcluded(),
		)
	if _, execErr := s.execBuilderContext(ctx, query); execErr != nil {
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

	query := querydsl.SelectFrom(metadataObjectVersions.table, metadataObjectVersions.selectItems()...).
		Where(querydsl.And(
			metadataObjectVersions.bucket.Eq(bucket),
			metadataObjectVersions.key.Eq(key),
			metadataObjectVersions.versionID.Eq(versionID),
		)).
		Limit(1)
	row, queryErr := s.queryRowBuilderContext(ctx, query)
	if queryErr != nil {
		return model.ObjectVersion{}, false, fmt.Errorf("get object version: %w", queryErr)
	}
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

	query := querydsl.SelectFrom(metadataObjectVersions.table, metadataObjectVersions.selectItems()...).
		Where(querydsl.And(metadataObjectVersions.bucket.Eq(bucket), metadataObjectVersions.key.Eq(key))).
		OrderBy(metadataObjectVersions.createdAt.Desc(), metadataObjectVersions.versionID.Desc())
	rows, err := s.queryBuilderContext(ctx, query)
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
	query := querydsl.DeleteFrom(metadataObjectVersions.table).
		Where(querydsl.And(
			metadataObjectVersions.bucket.Eq(bucket),
			metadataObjectVersions.key.Eq(key),
			metadataObjectVersions.versionID.Eq(versionID),
		))
	result, err := s.execBuilderContext(ctx, query)
	if err != nil {
		return false, fmt.Errorf("delete object version: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete object version rows: %w", err)
	}
	return affected > 0, nil
}
