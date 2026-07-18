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

func (s *SQLMetadata) UpsertIndexJob(ctx context.Context, job model.IndexJob) (model.IndexJob, error) {
	job, err := prepareIndexJob(job)
	if err != nil {
		return model.IndexJob{}, err
	}
	query := querydsl.InsertInto(metadataIndexJobs.schema).
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
	if _, execErr := dbx.Exec(ensureContext(ctx), s.dbxDB, query); execErr != nil {
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

	return getRepositoryByKey[model.IndexJob](
		ctx,
		s.repos.indexJobs,
		repositoryx.KeySet(repositoryx.Part(metadataIndexJobs.id, id)),
		"query index job",
	)
}

func (s *SQLMetadata) ListIndexJobs(ctx context.Context, status string, limit int) (*collectionlist.List[model.IndexJob], error) {
	status = strings.TrimSpace(status)
	specs := []repositoryx.Spec{}
	if status != "" {
		specs = append(specs, repositoryx.Where(metadataIndexJobs.status.Eq(status)))
	}
	specs = append(
		specs,
		repositoryx.OrderBy(metadataIndexJobs.availableAt.Asc(), metadataIndexJobs.createdAt.Asc()),
		repositoryx.Limit(normalizeListLimit(limit)),
	)
	jobs, err := s.repos.indexJobs.ListSpec(ctx, specs...)
	if err != nil {
		return nil, fmt.Errorf("list index jobs: %w", err)
	}
	return jobs, nil
}

func (s *SQLMetadata) DeleteIndexJob(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	result, err := s.repos.indexJobs.DeleteByKeySet(ctx, repositoryx.KeySet(repositoryx.Part(metadataIndexJobs.id, id)))
	if err != nil {
		return false, fmt.Errorf("delete index job: %w", err)
	}
	return hasAffectedRow(result, "delete index job")
}
