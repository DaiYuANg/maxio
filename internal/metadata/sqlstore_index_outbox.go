package metadata

import (
	"context"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/querydsl"
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
	stored, found, err := s.GetIndexOutboxEvent(ctx, event.ID)
	if err != nil {
		return model.IndexOutboxEvent{}, err
	}
	if !found {
		return model.IndexOutboxEvent{}, ErrObjectNotFound
	}
	return stored, nil
}

func (s *SQLMetadata) GetIndexOutboxEvent(ctx context.Context, id string) (model.IndexOutboxEvent, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexOutboxEvent{}, false, ErrBadRequest
	}

	option, err := s.repos.indexOutbox.GetByKeyOption(ctx, metadataKey(metadataIndexOutbox.id, id))
	if err != nil {
		return model.IndexOutboxEvent{}, false, fmt.Errorf("query index outbox event: %w", err)
	}
	event, found := option.Get()
	return event, found, nil
}

func (s *SQLMetadata) ListIndexOutboxEvents(ctx context.Context, status string, limit int) (*collectionlist.List[model.IndexOutboxEvent], error) {
	status = strings.TrimSpace(status)
	query := querydsl.SelectFrom(metadataIndexOutbox.schema, metadataIndexOutbox.selectItems()...).
		OrderBy(metadataIndexOutbox.availableAt.Asc(), metadataIndexOutbox.createdAt.Asc()).
		Limit(normalizeListLimit(limit))
	if status != "" {
		query.Where(metadataIndexOutbox.status.Eq(status))
	}
	events, err := s.repos.indexOutbox.List(ctx, query)
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
	result, err := s.repos.indexOutbox.DeleteByKey(ctx, metadataKey(metadataIndexOutbox.id, id))
	if err != nil {
		return false, fmt.Errorf("delete index outbox event: %w", err)
	}
	return hasAffectedRow(result, "delete index outbox event")
}
