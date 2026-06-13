package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

func (s *SQLMetadata) UpsertIndexOutboxEvent(ctx context.Context, event model.IndexOutboxEvent) (model.IndexOutboxEvent, error) {
	event, err := prepareIndexOutboxEvent(event)
	if err != nil {
		return model.IndexOutboxEvent{}, err
	}
	if _, execErr := s.execContext(
		ctx,
		sqlStoreIndexOutboxUpsertSQL,
		event.ID,
		event.EventType,
		event.Bucket,
		event.Key,
		event.VersionID,
		event.Payload,
		event.Status,
		event.Attempts,
		event.Error,
		event.AvailableAt.UnixNano(),
		event.CreatedAt.UnixNano(),
		event.UpdatedAt.UnixNano(),
	); execErr != nil {
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

	row := s.queryRowContext(
		ctx,
		`SELECT `+sqlStoreIndexOutboxColumns+`
		   FROM metadata_index_outbox
		  WHERE event_id = ?
		  LIMIT 1`,
		id,
	)
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
	return listSQLIndexQueue(
		ctx,
		s,
		sqlStoreIndexOutboxColumns,
		"metadata_index_outbox",
		status,
		limit,
		"index outbox",
		scanIndexOutboxEvent,
	)
}

func (s *SQLMetadata) DeleteIndexOutboxEvent(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	return deleteByID(ctx, s, `DELETE FROM metadata_index_outbox WHERE event_id = ?`, id, "index outbox event")
}
