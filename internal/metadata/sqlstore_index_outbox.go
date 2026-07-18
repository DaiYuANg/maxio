package metadata

import (
	"context"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) UpsertIndexOutboxEvent(ctx context.Context, event model.IndexOutboxEvent) (model.IndexOutboxEvent, error) {
	event, err := prepareIndexOutboxEvent(event)
	if err != nil {
		return model.IndexOutboxEvent{}, err
	}
	if err := execRepositoryUpsert(
		ctx,
		s.repos.indexOutbox,
		metadataIndexOutbox.schema,
		&event,
		"map index outbox insert assignments",
		"upsert index outbox event",
		collectionlist.NewList[querydsl.Expression](metadataIndexOutbox.id),
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
	); err != nil {
		return model.IndexOutboxEvent{}, err
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
	return deleteRepositoryByKey[model.IndexOutboxEvent](
		ctx,
		s.repos.indexOutbox,
		repositoryx.KeySet(repositoryx.Part(metadataIndexOutbox.id, id)),
		"delete index outbox event",
	)
}
