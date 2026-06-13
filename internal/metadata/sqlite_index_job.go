package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

func (s *SQLiteMetadata) UpsertIndexJob(ctx context.Context, job model.IndexJob) (model.IndexJob, error) {
	job, err := prepareIndexJob(job)
	if err != nil {
		return model.IndexJob{}, err
	}
	if _, execErr := s.execContext(
		ctx,
		sqliteIndexJobUpsertSQL,
		job.ID,
		job.Kind,
		job.Bucket,
		job.Key,
		job.VersionID,
		job.Status,
		job.Attempts,
		job.Error,
		job.AvailableAt.UnixNano(),
		unixNanoOrNil(job.StartedAt),
		unixNanoOrNil(job.FinishedAt),
		job.CreatedAt.UnixNano(),
		job.UpdatedAt.UnixNano(),
	); execErr != nil {
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

func (s *SQLiteMetadata) GetIndexJob(ctx context.Context, id string) (model.IndexJob, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexJob{}, false, ErrBadRequest
	}

	row := s.queryRowContext(
		ctx,
		`SELECT `+sqliteIndexJobColumns+`
		   FROM metadata_index_jobs
		  WHERE job_id = ?
		  LIMIT 1`,
		id,
	)
	job, err := scanIndexJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.IndexJob{}, false, nil
	}
	if err != nil {
		return model.IndexJob{}, false, fmt.Errorf("get index job: %w", err)
	}
	return job, true, nil
}

func (s *SQLiteMetadata) ListIndexJobs(ctx context.Context, status string, limit int) ([]model.IndexJob, error) {
	return listSQLiteIndexQueue(
		ctx,
		s,
		sqliteIndexJobColumns,
		"metadata_index_jobs",
		status,
		limit,
		"index jobs",
		scanIndexJob,
	)
}

func (s *SQLiteMetadata) DeleteIndexJob(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	return deleteByID(ctx, s, `DELETE FROM metadata_index_jobs WHERE job_id = ?`, id, "index job")
}
