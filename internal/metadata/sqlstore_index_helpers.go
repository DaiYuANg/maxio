package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/arcgolabs/dbx/querydsl"
	"github.com/lyonbrown4d/maxio/internal/model"
)

type sqlScanner interface {
	Scan(dest ...any) error
}

func scanIndexDocument(scanner sqlScanner) (model.IndexDocument, error) {
	var (
		document model.IndexDocument
		indexed  sql.NullInt64
		created  int64
		updated  int64
	)
	if err := scanner.Scan(
		&document.ID,
		&document.Bucket,
		&document.Key,
		&document.VersionID,
		&document.Digest,
		&document.State,
		&document.Error,
		&indexed,
		&created,
		&updated,
	); err != nil {
		return model.IndexDocument{}, fmt.Errorf("scan index document: %w", err)
	}
	document.IndexedAt = unixNanoToTimeOpt(indexed)
	document.CreatedAt = unixNanoToTime(created)
	document.UpdatedAt = unixNanoToTime(updated)
	return document, nil
}

func scanIndexJob(scanner sqlScanner) (model.IndexJob, error) {
	var (
		job       model.IndexJob
		started   sql.NullInt64
		finished  sql.NullInt64
		created   int64
		updated   int64
		available int64
	)
	if err := scanner.Scan(
		&job.ID,
		&job.Kind,
		&job.Bucket,
		&job.Key,
		&job.VersionID,
		&job.Status,
		&job.Attempts,
		&job.Error,
		&available,
		&started,
		&finished,
		&created,
		&updated,
	); err != nil {
		return model.IndexJob{}, fmt.Errorf("scan index job: %w", err)
	}
	job.AvailableAt = unixNanoToTime(available)
	job.StartedAt = unixNanoToTimeOpt(started)
	job.FinishedAt = unixNanoToTimeOpt(finished)
	job.CreatedAt = unixNanoToTime(created)
	job.UpdatedAt = unixNanoToTime(updated)
	return job, nil
}

func scanIndexOutboxEvent(scanner sqlScanner) (model.IndexOutboxEvent, error) {
	var (
		event     model.IndexOutboxEvent
		available int64
		created   int64
		updated   int64
	)
	if err := scanner.Scan(
		&event.ID,
		&event.EventType,
		&event.Bucket,
		&event.Key,
		&event.VersionID,
		&event.Payload,
		&event.Status,
		&event.Attempts,
		&event.Error,
		&available,
		&created,
		&updated,
	); err != nil {
		return model.IndexOutboxEvent{}, fmt.Errorf("scan index outbox event: %w", err)
	}
	event.AvailableAt = unixNanoToTime(available)
	event.CreatedAt = unixNanoToTime(created)
	event.UpdatedAt = unixNanoToTime(updated)
	return event, nil
}

func prepareIndexDocument(document model.IndexDocument) (model.IndexDocument, error) {
	document.ID = strings.TrimSpace(document.ID)
	document.Bucket = strings.TrimSpace(document.Bucket)
	document.Key = strings.TrimSpace(document.Key)
	document.VersionID = strings.TrimSpace(document.VersionID)
	document.Digest = strings.TrimSpace(document.Digest)
	document.State = strings.TrimSpace(document.State)
	if document.ID == "" {
		document.ID = indexEntityID(document.Bucket, document.Key, document.VersionID)
	}
	if document.ID == "" || document.Bucket == "" || document.Key == "" || document.VersionID == "" {
		return model.IndexDocument{}, ErrBadRequest
	}
	if document.State == "" {
		document.State = model.IndexDocumentStatePending
	}
	now := time.Now().UTC()
	if document.CreatedAt.IsZero() {
		document.CreatedAt = now
	}
	document.UpdatedAt = now
	return document, nil
}

func prepareIndexJob(job model.IndexJob) (model.IndexJob, error) {
	job.ID = strings.TrimSpace(job.ID)
	job.Kind = strings.TrimSpace(job.Kind)
	job.Bucket = strings.TrimSpace(job.Bucket)
	job.Key = strings.TrimSpace(job.Key)
	job.VersionID = strings.TrimSpace(job.VersionID)
	job.Status = normalizeIndexQueueStatus(job.Status, model.IndexJobStatusQueued)
	if invalidIndexQueueIdentity(job.ID, job.Kind) {
		return model.IndexJob{}, ErrBadRequest
	}
	job.AvailableAt, job.CreatedAt, job.UpdatedAt = indexQueueTimes(job.AvailableAt, job.CreatedAt)
	return job, nil
}

func prepareIndexOutboxEvent(event model.IndexOutboxEvent) (model.IndexOutboxEvent, error) {
	event.ID = strings.TrimSpace(event.ID)
	event.EventType = strings.TrimSpace(event.EventType)
	event.Bucket = strings.TrimSpace(event.Bucket)
	event.Key = strings.TrimSpace(event.Key)
	event.VersionID = strings.TrimSpace(event.VersionID)
	event.Status = normalizeIndexQueueStatus(event.Status, model.IndexOutboxStatusPending)
	if invalidIndexQueueIdentity(event.ID, event.EventType) {
		return model.IndexOutboxEvent{}, ErrBadRequest
	}
	event.AvailableAt, event.CreatedAt, event.UpdatedAt = indexQueueTimes(event.AvailableAt, event.CreatedAt)
	return event, nil
}

func normalizeIndexQueueStatus(status, fallback string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return fallback
	}
	return status
}

func invalidIndexQueueIdentity(id, kind string) bool {
	return id == "" || kind == ""
}

func indexQueueTimes(availableAt, createdAt time.Time) (time.Time, time.Time, time.Time) {
	now := time.Now().UTC()
	if availableAt.IsZero() {
		availableAt = now
	}
	if createdAt.IsZero() {
		createdAt = now
	}
	return availableAt, createdAt, now
}

func listSQLIndexQueue[T any](
	ctx context.Context,
	store *SQLMetadata,
	query querydsl.Builder,
	label string,
	scan func(sqlScanner) (T, error),
) ([]T, error) {
	rows, err := store.queryBuilderContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && store.logger != nil {
			store.logger.Error("close sql metadata rows", "rows", label, "error", closeErr)
		}
	}()

	items := make([]T, 0)
	for rows.Next() {
		item, scanErr := scan(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate %s: %w", label, rowsErr)
	}
	return items, nil
}
