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

func (s *SQLMetadata) UpsertIndexOutboxEvent(ctx context.Context, event model.IndexOutboxEvent) (model.IndexOutboxEvent, error) {
	event, err := prepareIndexOutboxEvent(event)
	if err != nil {
		return model.IndexOutboxEvent{}, err
	}
	assignments, err := s.repos.indexOutbox.Mapper().InsertAssignmentsWithID(ctx, metadataIndexOutbox.schema, &event, nil)
	if err != nil {
		return model.IndexOutboxEvent{}, fmt.Errorf("map index outbox insert assignments: %w", err)
	}
	query := querydsl.InsertInto(metadataIndexOutbox.schema).
		ValuesList(assignments).
		OnConflict(metadataIndexOutbox.id).
		DoUpdateSet(
			metadataIndexOutbox.eventType.SetExcluded(),
			metadataIndexOutbox.bucket.SetExcluded(),
			metadataIndexOutbox.key.SetExcluded(),
			metadataIndexOutbox.versionID.SetExcluded(),
			metadataIndexOutbox.payload.SetExcluded(),
			metadataIndexOutbox.status.SetExcluded(),
			metadataIndexOutbox.attempts.SetExcluded(),
			metadataIndexOutbox.errorText.SetExcluded(),
			metadataIndexOutbox.availableAt.SetExcluded(),
			metadataIndexOutbox.updatedAt.SetExcluded(),
		)
	if _, execErr := dbx.Exec(ensureContext(ctx), s.dbxDB, query); execErr != nil {
		return model.IndexOutboxEvent{}, fmt.Errorf("upsert index outbox event: %w", execErr)
	}
	return requireStoredEntity(s.GetIndexOutboxEvent(ctx, event.ID))
}

func (s *SQLMetadata) GetIndexOutboxEvent(ctx context.Context, id string) (model.IndexOutboxEvent, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexOutboxEvent{}, false, ErrBadRequest
	}

	return getRepositoryByKey[model.IndexOutboxEvent](
		ctx,
		s.repos.indexOutbox,
		repositoryx.KeySet(repositoryx.Part(metadataIndexOutbox.id, id)),
		"query index outbox event",
	)
}

func (s *SQLMetadata) ListIndexOutboxEvents(ctx context.Context, status string, limit int) (*collectionlist.List[model.IndexOutboxEvent], error) {
	status = strings.TrimSpace(status)
	var predicate querydsl.Predicate
	if status != "" {
		predicate = metadataIndexOutbox.status.Eq(status)
	}
	specs := repositorySpecs(
		optionalWhereSpec(predicate),
		repositoryx.OrderBy(metadataIndexOutbox.availableAt.Asc(), metadataIndexOutbox.createdAt.Asc()),
		repositoryx.Limit(normalizeListLimit(limit)),
	)
	events, err := s.repos.indexOutbox.ListSpec(ctx, specs...)
	if err != nil {
		return nil, fmt.Errorf("list index outbox: %w", err)
	}
	return events, nil
}

func (s *SQLMetadata) DeleteIndexOutboxEvent(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	result, err := s.repos.indexOutbox.DeleteByKeySet(ctx, repositoryx.KeySet(repositoryx.Part(metadataIndexOutbox.id, id)))
	if err != nil {
		return false, fmt.Errorf("delete index outbox event: %w", err)
	}
	return hasAffectedRow(result, "delete index outbox event")
}
