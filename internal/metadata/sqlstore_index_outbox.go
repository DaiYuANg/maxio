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

func (s *SQLMetadata) UpsertIndexOutboxEvent(ctx context.Context, event model.IndexOutboxEvent) (model.IndexOutboxEvent, error) {
	event, err := prepareIndexOutboxEvent(event)
	if err != nil {
		return model.IndexOutboxEvent{}, err
	}
	query := querydsl.InsertInto(metadataIndexOutbox.table).
		Values(
			metadataIndexOutbox.id.Set(event.ID),
			metadataIndexOutbox.eventType.Set(event.EventType),
			metadataIndexOutbox.bucket.Set(event.Bucket),
			metadataIndexOutbox.key.Set(event.Key),
			metadataIndexOutbox.versionID.Set(event.VersionID),
			metadataIndexOutbox.payload.Set(event.Payload),
			metadataIndexOutbox.status.Set(event.Status),
			metadataIndexOutbox.attempts.Set(event.Attempts),
			metadataIndexOutbox.errorText.Set(event.Error),
			metadataIndexOutbox.availableAt.Set(event.AvailableAt.UnixNano()),
			metadataIndexOutbox.createdAt.Set(event.CreatedAt.UnixNano()),
			metadataIndexOutbox.updatedAt.Set(event.UpdatedAt.UnixNano()),
		).
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
	if _, execErr := s.execBuilderContext(ctx, query); execErr != nil {
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

	query := querydsl.SelectFrom(metadataIndexOutbox.table, metadataIndexOutbox.selectItems()...).
		Where(metadataIndexOutbox.id.Eq(id)).
		Limit(1)
	row, err := s.queryRowBuilderContext(ctx, query)
	if err != nil {
		return model.IndexOutboxEvent{}, false, fmt.Errorf("get index outbox event: %w", err)
	}
	event, err := scanIndexOutboxEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.IndexOutboxEvent{}, false, nil
	}
	if err != nil {
		return model.IndexOutboxEvent{}, false, fmt.Errorf("get index outbox event: %w", err)
	}
	return event, true, nil
}

func (s *SQLMetadata) ListIndexOutboxEvents(ctx context.Context, status string, limit int) ([]model.IndexOutboxEvent, error) {
	status = strings.TrimSpace(status)
	query := querydsl.SelectFrom(metadataIndexOutbox.table, metadataIndexOutbox.selectItems()...).
		OrderBy(metadataIndexOutbox.availableAt.Asc(), metadataIndexOutbox.createdAt.Asc()).
		Limit(normalizeListLimit(limit))
	if status != "" {
		query.Where(metadataIndexOutbox.status.Eq(status))
	}
	return listSQLIndexQueue(
		ctx,
		s,
		query,
		"index outbox",
		scanIndexOutboxEvent,
	)
}

func (s *SQLMetadata) DeleteIndexOutboxEvent(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	query := querydsl.DeleteFrom(metadataIndexOutbox.table).Where(metadataIndexOutbox.id.Eq(id))
	result, err := s.execBuilderContext(ctx, query)
	if err != nil {
		return false, fmt.Errorf("delete index outbox event: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete index outbox event rows: %w", err)
	}
	return affected > 0, nil
}
