package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

const sqliteIndexDocumentColumns = `document_id, bucket, object_key, version_id, digest, state, error, indexed_at, created_at, updated_at`

const sqliteIndexJobColumns = `job_id, kind, bucket, object_key, version_id, status, attempts, error,
available_at, started_at, finished_at, created_at, updated_at`

const sqliteIndexOutboxColumns = `event_id, event_type, bucket, object_key, version_id, payload, status,
attempts, error, available_at, created_at, updated_at`

const sqliteIndexDocumentUpsertSQL = `INSERT INTO metadata_index_documents (
	document_id, bucket, object_key, version_id, digest, state, error, indexed_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(document_id) DO UPDATE SET
	bucket = excluded.bucket,
	object_key = excluded.object_key,
	version_id = excluded.version_id,
	digest = excluded.digest,
	state = excluded.state,
	error = excluded.error,
	indexed_at = excluded.indexed_at,
	updated_at = excluded.updated_at`

const sqliteIndexJobUpsertSQL = `INSERT INTO metadata_index_jobs (
	job_id, kind, bucket, object_key, version_id, status, attempts, error,
	available_at, started_at, finished_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(job_id) DO UPDATE SET
	kind = excluded.kind,
	bucket = excluded.bucket,
	object_key = excluded.object_key,
	version_id = excluded.version_id,
	status = excluded.status,
	attempts = excluded.attempts,
	error = excluded.error,
	available_at = excluded.available_at,
	started_at = excluded.started_at,
	finished_at = excluded.finished_at,
	updated_at = excluded.updated_at`

const sqliteIndexOutboxUpsertSQL = `INSERT INTO metadata_index_outbox (
	event_id, event_type, bucket, object_key, version_id, payload, status,
	attempts, error, available_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(event_id) DO UPDATE SET
	event_type = excluded.event_type,
	bucket = excluded.bucket,
	object_key = excluded.object_key,
	version_id = excluded.version_id,
	payload = excluded.payload,
	status = excluded.status,
	attempts = excluded.attempts,
	error = excluded.error,
	available_at = excluded.available_at,
	updated_at = excluded.updated_at`

func (s *SQLiteMetadata) UpsertIndexDocument(ctx context.Context, document model.IndexDocument) (model.IndexDocument, error) {
	document, err := prepareIndexDocument(document)
	if err != nil {
		return model.IndexDocument{}, err
	}
	if _, execErr := s.execContext(
		ctx,
		sqliteIndexDocumentUpsertSQL,
		document.ID,
		document.Bucket,
		document.Key,
		document.VersionID,
		document.Digest,
		document.State,
		document.Error,
		unixNanoOrNil(document.IndexedAt),
		document.CreatedAt.UnixNano(),
		document.UpdatedAt.UnixNano(),
	); execErr != nil {
		return model.IndexDocument{}, fmt.Errorf("upsert index document: %w", execErr)
	}
	stored, found, err := s.GetIndexDocument(ctx, document.ID)
	if err != nil {
		return model.IndexDocument{}, err
	}
	if !found {
		return model.IndexDocument{}, ErrObjectNotFound
	}
	return stored, nil
}

func (s *SQLiteMetadata) GetIndexDocument(ctx context.Context, id string) (model.IndexDocument, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.IndexDocument{}, false, ErrBadRequest
	}

	row := s.queryRowContext(
		ctx,
		`SELECT `+sqliteIndexDocumentColumns+`
		   FROM metadata_index_documents
		  WHERE document_id = ?
		  LIMIT 1`,
		id,
	)
	document, err := scanIndexDocument(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.IndexDocument{}, false, nil
	}
	if err != nil {
		return model.IndexDocument{}, false, fmt.Errorf("get index document: %w", err)
	}
	return document, true, nil
}

func (s *SQLiteMetadata) ListIndexDocuments(ctx context.Context, bucket, prefix string) ([]model.IndexDocument, error) {
	bucket = strings.TrimSpace(bucket)
	prefix = strings.TrimSpace(prefix)
	rows, err := s.queryContext(
		ctx,
		`SELECT `+sqliteIndexDocumentColumns+`
		   FROM metadata_index_documents
		  WHERE (? = '' OR bucket = ?) AND (? = '' OR object_key LIKE ?)
		  ORDER BY bucket ASC, object_key ASC, version_id ASC`,
		bucket,
		bucket,
		prefix,
		prefixPattern(prefix),
	)
	if err != nil {
		return nil, fmt.Errorf("query index documents: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
			s.logger.Error("close sqlite rows", "rows", "index documents", "error", closeErr)
		}
	}()

	documents := make([]model.IndexDocument, 0)
	for rows.Next() {
		document, err := scanIndexDocument(rows)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate index documents: %w", err)
	}
	return documents, nil
}

func (s *SQLiteMetadata) DeleteIndexDocument(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}
	return deleteByID(ctx, s, `DELETE FROM metadata_index_documents WHERE document_id = ?`, id, "index document")
}
