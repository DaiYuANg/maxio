package metadata

import (
	"context"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
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
	assignments, err := repositoryInsertAssignments(ctx, s.repos.objectRecords, metadataObjectRecords.schema, &record, "map object record insert assignments")
	if err != nil {
		return model.ObjectRecord{}, err
	}
	query := querydsl.InsertInto(metadataObjectRecords.schema).
		ValuesList(assignments).
		OnConflict(metadataObjectRecords.bucket, metadataObjectRecords.key).
		DoUpdateSet(
			metadataObjectRecords.currentVersionID.SetExcluded(),
			metadataObjectRecords.deleted.SetExcluded(),
			metadataObjectRecords.updatedAt.SetExcluded(),
		)
	if _, execErr := dbx.Exec(ensureContext(ctx), s.dbxDB, query); execErr != nil {
		return model.ObjectRecord{}, fmt.Errorf("upsert object record: %w", execErr)
	}
	return requireStoredEntity(s.GetObjectRecord(ctx, record.Bucket, record.Key))
}

func (s *SQLMetadata) GetObjectRecord(ctx context.Context, bucket, key string) (model.ObjectRecord, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectRecord{}, false, ErrBadRequest
	}

	return getRepositoryByKey[model.ObjectRecord](
		ctx,
		s.repos.objectRecords,
		repositoryx.KeySet(repositoryx.Part(metadataObjectRecords.bucket, bucket), repositoryx.Part(metadataObjectRecords.key, key)),
		"query object record",
	)
}

func (s *SQLMetadata) DeleteObjectRecord(ctx context.Context, bucket, key string) (bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return false, ErrBadRequest
	}
	return deleteRepositoryByKey[model.ObjectRecord](
		ctx,
		s.repos.objectRecords,
		repositoryx.KeySet(repositoryx.Part(metadataObjectRecords.bucket, bucket), repositoryx.Part(metadataObjectRecords.key, key)),
		"delete object record",
	)
}

func (s *SQLMetadata) UpsertObjectVersion(ctx context.Context, version model.ObjectVersion) (model.ObjectVersion, error) {
	version, err := prepareDBObjectVersion(version)
	if err != nil {
		return model.ObjectVersion{}, err
	}
	if ensureErr := s.ensureBucket(ctx, version.Bucket); ensureErr != nil {
		return model.ObjectVersion{}, ensureErr
	}
	assignments, err := repositoryInsertAssignments(ctx, s.repos.objectVersions, metadataObjectVersions.schema, &version, "map object version insert assignments")
	if err != nil {
		return model.ObjectVersion{}, err
	}
	query := querydsl.InsertInto(metadataObjectVersions.schema).
		ValuesList(assignments).
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
	if _, execErr := dbx.Exec(ensureContext(ctx), s.dbxDB, query); execErr != nil {
		return model.ObjectVersion{}, fmt.Errorf("upsert object version: %w", execErr)
	}
	return requireStoredEntity(s.GetObjectVersion(ctx, version.Bucket, version.Key, version.VersionID))
}

func (s *SQLMetadata) GetObjectVersion(ctx context.Context, bucket, key, versionID string) (model.ObjectVersion, bool, error) {
	bucket, key, versionID = trimObjectVersionKey(bucket, key, versionID)
	if bucket == "" || key == "" || versionID == "" {
		return model.ObjectVersion{}, false, ErrBadRequest
	}

	return getRepositoryByKey[model.ObjectVersion](
		ctx,
		s.repos.objectVersions,
		repositoryx.KeySet(
			repositoryx.Part(metadataObjectVersions.bucket, bucket),
			repositoryx.Part(metadataObjectVersions.key, key),
			repositoryx.Part(metadataObjectVersions.versionID, versionID),
		),
		"query object version",
	)
}

func (s *SQLMetadata) ListObjectVersions(ctx context.Context, bucket, key string) (*collectionlist.List[model.ObjectVersion], error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return nil, ErrBadRequest
	}

	versions, err := s.repos.objectVersions.ListSpec(
		ctx,
		repositoryx.Where(querydsl.And(metadataObjectVersions.bucket.Eq(bucket), metadataObjectVersions.key.Eq(key))),
		repositoryx.OrderBy(metadataObjectVersions.createdAt.Desc(), metadataObjectVersions.versionID.Desc()),
	)
	if err != nil {
		return nil, fmt.Errorf("list object versions: %w", err)
	}
	return versions, nil
}

func (s *SQLMetadata) DeleteObjectVersion(ctx context.Context, bucket, key, versionID string) (bool, error) {
	bucket, key, versionID = trimObjectVersionKey(bucket, key, versionID)
	if bucket == "" || key == "" || versionID == "" {
		return false, ErrBadRequest
	}
	return deleteRepositoryByKey[model.ObjectVersion](
		ctx,
		s.repos.objectVersions,
		repositoryx.KeySet(
			repositoryx.Part(metadataObjectVersions.bucket, bucket),
			repositoryx.Part(metadataObjectVersions.key, key),
			repositoryx.Part(metadataObjectVersions.versionID, versionID),
		),
		"delete object version",
	)
}
