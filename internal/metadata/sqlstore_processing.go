package metadata

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) UpsertProcessingRecord(ctx context.Context, record model.ProcessingRecord) (model.ProcessingRecord, error) {
	record, err := prepareProcessingRecord(record)
	if err != nil {
		return model.ProcessingRecord{}, err
	}
	assignments, err := repositoryInsertAssignments(ctx, s.repos.processingRecords, metadataProcessingRecords.schema, &record, "map processing record insert assignments")
	if err != nil {
		return model.ProcessingRecord{}, err
	}
	query := querydsl.InsertInto(metadataProcessingRecords.schema).
		ValuesList(assignments).
		OnConflict(metadataProcessingRecords.id).
		DoUpdateSet(
			metadataProcessingRecords.bucket.SetExcluded(),
			metadataProcessingRecords.key.SetExcluded(),
			metadataProcessingRecords.versionID.SetExcluded(),
			metadataProcessingRecords.digest.SetExcluded(),
			metadataProcessingRecords.mode.SetExcluded(),
			metadataProcessingRecords.status.SetExcluded(),
			metadataProcessingRecords.errorText.SetExcluded(),
			metadataProcessingRecords.results.SetExcluded(),
			metadataProcessingRecords.updatedAt.SetExcluded(),
		)
	if _, execErr := dbx.Exec(ensureContext(ctx), s.dbxDB, query); execErr != nil {
		return model.ProcessingRecord{}, fmt.Errorf("upsert processing record: %w", execErr)
	}
	return requireStoredEntity(s.GetProcessingRecord(ctx, record.Bucket, record.Key, record.VersionID, record.Digest))
}

func (s *SQLMetadata) GetProcessingRecord(ctx context.Context, bucket, key, versionID, digest string) (model.ProcessingRecord, bool, error) {
	id := processingRecordID(bucket, key, versionID, digest)
	if id == "" {
		return model.ProcessingRecord{}, false, ErrBadRequest
	}
	return getRepositoryByKey[model.ProcessingRecord](
		ctx,
		s.repos.processingRecords,
		repositoryx.KeySet(repositoryx.Part(metadataProcessingRecords.id, id)),
		"query processing record",
	)
}

func (s *SQLMetadata) ListProcessingRecords(ctx context.Context, status string, limit int) (*collectionlist.List[model.ProcessingRecord], error) {
	status = strings.TrimSpace(status)
	limit = normalizeListLimit(limit)
	var predicate querydsl.Predicate
	if status != "" {
		predicate = metadataProcessingRecords.status.Eq(status)
	}
	specs := repositorySpecs(
		optionalWhereSpec(predicate),
		repositoryx.OrderBy(metadataProcessingRecords.updatedAt.Desc(), metadataProcessingRecords.id.Asc()),
		repositoryx.Limit(limit),
	)
	records, err := s.repos.processingRecords.ListSpec(ctx, specs...)
	if err != nil {
		return nil, fmt.Errorf("list processing records: %w", err)
	}
	return records, nil
}

func (s *SQLMetadata) DeleteProcessingRecord(ctx context.Context, bucket, key, versionID, digest string) (bool, error) {
	id := processingRecordID(bucket, key, versionID, digest)
	if id == "" {
		return false, ErrBadRequest
	}
	return deleteRepositoryByKey[model.ProcessingRecord](
		ctx,
		s.repos.processingRecords,
		repositoryx.KeySet(repositoryx.Part(metadataProcessingRecords.id, id)),
		"delete processing record",
	)
}

func prepareProcessingRecord(record model.ProcessingRecord) (model.ProcessingRecord, error) {
	record.Mode = strings.TrimSpace(strings.ToLower(record.Mode))
	record.Status = strings.TrimSpace(strings.ToLower(record.Status))
	record.ID = processingRecordID(record.Bucket, record.Key, record.VersionID, record.Digest)
	if record.ID == "" || record.Mode == "" || record.Status == "" {
		return model.ProcessingRecord{}, ErrBadRequest
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	return record, nil
}

func processingRecordID(bucket, key, versionID, digest string) string {
	identity := versionID
	if identity == "" {
		identity = digest
	}
	if strings.TrimSpace(bucket) == "" || key == "" || identity == "" {
		return ""
	}
	return processingRecordIDPart(bucket) + "." + processingRecordIDPart(key) + "." + processingRecordIDPart(identity)
}

func processingRecordIDPart(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}
