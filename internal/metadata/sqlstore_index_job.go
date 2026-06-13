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

func (s *SQLMetadata) UpsertIndexJob(ctx context.Context, job model.IndexJob) (model.IndexJob, error) {
	job, err := prepareIndexJob(job)
	if err != nil {
		return model.IndexJob{}, err
	}
	query := querydsl.InsertInto(metadataIndexJobs.table).
		Values(
			metadataIndexJobs.id.Set(job.ID),
			metadataIndexJobs.kind.Set(job.Kind),
			metadataIndexJobs.bucket.Set(job.Bucket),
			metadataIndexJobs.key.Set(job.Key),
			metadataIndexJobs.versionID.Set(job.VersionID),
			metadataIndexJobs.status.Set(job.Status),
			metadataIndexJobs.attempts.Set(job.Attempts),
			metadataIndexJobs.errorText.Set(job.Error),
			metadataIndexJobs.availableAt.Set(job.AvailableAt.UnixNano()),
			metadataIndexJobs.startedAt.Set(unixNanoOrNil(job.StartedAt)),
			metadataIndexJobs.finishedAt.Set(unixNanoOrNil(job.FinishedAt)),
			metadataIndexJobs.createdAt.Set(job.CreatedAt.UnixNano()),
			metadataIndexJobs.updatedAt.Set(job.UpdatedAt.UnixNano()),
		).
		OnConflict(metadataIndexJobs.id).
		DoUpdateSet(
			metadataIndexJobs.kind.SetExcluded(),
			metadataIndexJobs.bucket.SetExcluded(),
			metadataIndexJobs.key.SetExcluded(),
			metadataIndexJobs.versionID.SetExcluded(),
			metadataIndexJobs.status.SetExcluded(),
			metadataIndexJobs.attempts.SetExcluded(),
			metadataIndexJobs.errorText.SetExcluded(),
			metadataIndexJobs.availableAt.SetExcluded(),
			metadataIndexJobs.startedAt.SetExcluded(),
			metadataIndexJobs.finishedAt.SetExcluded(),
			metadataIndexJobs.updatedAt.SetExcluded(),
		)
	if _, execErr := s.execBuilderContext(ctx, query); execErr != nil {
		return model.IndexJob{}, fmt.Errorf("upsert index job: %w", execErr)
	}
	stored, found, err := s.GetIndexJob(ctx, job.ID)
	if err != nil {
		return model.IndexJob{}, err
	}
	if !found {
		return model.IndexJob{}, ErrObjectNotFound
	}
	return stored, nil
}

func (s *SQLMetadata) GetIndexJob(ctx context.Context, id string) (model.IndexJob, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexJob{}, false, ErrBadRequest
	}

	query := querydsl.SelectFrom(metadataIndexJobs.table, metadataIndexJobs.selectItems()...).
		Where(metadataIndexJobs.id.Eq(id)).
		Limit(1)
	row, err := s.queryRowBuilderContext(ctx, query)
	if err != nil {
		return model.IndexJob{}, false, fmt.Errorf("get index job: %w", err)
	}
	job, err := scanIndexJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.IndexJob{}, false, nil
	}
	if err != nil {
		return model.IndexJob{}, false, fmt.Errorf("get index job: %w", err)
	}
	return job, true, nil
}

func (s *SQLMetadata) ListIndexJobs(ctx context.Context, status string, limit int) ([]model.IndexJob, error) {
	status = strings.TrimSpace(status)
	query := querydsl.SelectFrom(metadataIndexJobs.table, metadataIndexJobs.selectItems()...).
		OrderBy(metadataIndexJobs.availableAt.Asc(), metadataIndexJobs.createdAt.Asc()).
		Limit(normalizeListLimit(limit))
	if status != "" {
		query.Where(metadataIndexJobs.status.Eq(status))
	}
	return listSQLIndexQueue(
		ctx,
		s,
		query,
		"index jobs",
		scanIndexJob,
	)
}

func (s *SQLMetadata) DeleteIndexJob(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	query := querydsl.DeleteFrom(metadataIndexJobs.table).Where(metadataIndexJobs.id.Eq(id))
	result, err := s.execBuilderContext(ctx, query)
	if err != nil {
		return false, fmt.Errorf("delete index job: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete index job rows: %w", err)
	}
	return affected > 0, nil
}
