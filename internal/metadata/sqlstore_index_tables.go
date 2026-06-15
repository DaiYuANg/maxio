package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	schemax "github.com/arcgolabs/dbx/schema"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var (
	metadataIndexDocuments = newMetadataIndexDocumentsTable()
	metadataIndexJobs      = newMetadataIndexJobsTable()
	metadataIndexOutbox    = newMetadataIndexOutboxTable()
)

type metadataIndexDocumentsTable struct {
	schema    metadataIndexDocumentsSchema
	id        columnx.Column[model.IndexDocument, string]
	bucket    columnx.Column[model.IndexDocument, string]
	key       columnx.Column[model.IndexDocument, string]
	versionID columnx.Column[model.IndexDocument, string]
	digest    columnx.Column[model.IndexDocument, string]
	state     columnx.Column[model.IndexDocument, string]
	errorText columnx.Column[model.IndexDocument, string]
	indexedAt columnx.Column[model.IndexDocument, any]
	createdAt columnx.Column[model.IndexDocument, int64]
	updatedAt columnx.Column[model.IndexDocument, int64]
}

type metadataIndexJobsTable struct {
	schema      metadataIndexJobsSchema
	id          columnx.Column[model.IndexJob, string]
	kind        columnx.Column[model.IndexJob, string]
	bucket      columnx.Column[model.IndexJob, string]
	key         columnx.Column[model.IndexJob, string]
	versionID   columnx.Column[model.IndexJob, string]
	status      columnx.Column[model.IndexJob, string]
	attempts    columnx.Column[model.IndexJob, int]
	errorText   columnx.Column[model.IndexJob, string]
	availableAt columnx.Column[model.IndexJob, int64]
	startedAt   columnx.Column[model.IndexJob, any]
	finishedAt  columnx.Column[model.IndexJob, any]
	createdAt   columnx.Column[model.IndexJob, int64]
	updatedAt   columnx.Column[model.IndexJob, int64]
}

type metadataIndexOutboxTable struct {
	schema      metadataIndexOutboxSchema
	id          columnx.Column[model.IndexOutboxEvent, string]
	eventType   columnx.Column[model.IndexOutboxEvent, string]
	bucket      columnx.Column[model.IndexOutboxEvent, string]
	key         columnx.Column[model.IndexOutboxEvent, string]
	versionID   columnx.Column[model.IndexOutboxEvent, string]
	payload     columnx.Column[model.IndexOutboxEvent, string]
	status      columnx.Column[model.IndexOutboxEvent, string]
	attempts    columnx.Column[model.IndexOutboxEvent, int]
	errorText   columnx.Column[model.IndexOutboxEvent, string]
	availableAt columnx.Column[model.IndexOutboxEvent, int64]
	createdAt   columnx.Column[model.IndexOutboxEvent, int64]
	updatedAt   columnx.Column[model.IndexOutboxEvent, int64]
}

type metadataIndexDocumentsSchema struct {
	schemax.Schema[model.IndexDocument]
	ID        columnx.Column[model.IndexDocument, string] `dbx:"document_id,pk"`
	Bucket    columnx.Column[model.IndexDocument, string] `dbx:"bucket"`
	Key       columnx.Column[model.IndexDocument, string] `dbx:"object_key"`
	VersionID columnx.Column[model.IndexDocument, string] `dbx:"version_id"`
	Digest    columnx.Column[model.IndexDocument, string] `dbx:"digest"`
	State     columnx.Column[model.IndexDocument, string] `dbx:"state"`
	Error     columnx.Column[model.IndexDocument, string] `dbx:"error"`
	IndexedAt columnx.Column[model.IndexDocument, any]    `dbx:"indexed_at"`
	CreatedAt columnx.Column[model.IndexDocument, int64]  `dbx:"created_at"`
	UpdatedAt columnx.Column[model.IndexDocument, int64]  `dbx:"updated_at"`
}

type metadataIndexJobsSchema struct {
	schemax.Schema[model.IndexJob]
	ID          columnx.Column[model.IndexJob, string] `dbx:"job_id,pk"`
	Kind        columnx.Column[model.IndexJob, string] `dbx:"kind"`
	Bucket      columnx.Column[model.IndexJob, string] `dbx:"bucket"`
	Key         columnx.Column[model.IndexJob, string] `dbx:"object_key"`
	VersionID   columnx.Column[model.IndexJob, string] `dbx:"version_id"`
	Status      columnx.Column[model.IndexJob, string] `dbx:"status"`
	Attempts    columnx.Column[model.IndexJob, int]    `dbx:"attempts"`
	Error       columnx.Column[model.IndexJob, string] `dbx:"error"`
	AvailableAt columnx.Column[model.IndexJob, int64]  `dbx:"available_at"`
	StartedAt   columnx.Column[model.IndexJob, any]    `dbx:"started_at"`
	FinishedAt  columnx.Column[model.IndexJob, any]    `dbx:"finished_at"`
	CreatedAt   columnx.Column[model.IndexJob, int64]  `dbx:"created_at"`
	UpdatedAt   columnx.Column[model.IndexJob, int64]  `dbx:"updated_at"`
}

type metadataIndexOutboxSchema struct {
	schemax.Schema[model.IndexOutboxEvent]
	ID          columnx.Column[model.IndexOutboxEvent, string] `dbx:"event_id,pk"`
	EventType   columnx.Column[model.IndexOutboxEvent, string] `dbx:"event_type"`
	Bucket      columnx.Column[model.IndexOutboxEvent, string] `dbx:"bucket"`
	Key         columnx.Column[model.IndexOutboxEvent, string] `dbx:"object_key"`
	VersionID   columnx.Column[model.IndexOutboxEvent, string] `dbx:"version_id"`
	Payload     columnx.Column[model.IndexOutboxEvent, string] `dbx:"payload"`
	Status      columnx.Column[model.IndexOutboxEvent, string] `dbx:"status"`
	Attempts    columnx.Column[model.IndexOutboxEvent, int]    `dbx:"attempts"`
	Error       columnx.Column[model.IndexOutboxEvent, string] `dbx:"error"`
	AvailableAt columnx.Column[model.IndexOutboxEvent, int64]  `dbx:"available_at"`
	CreatedAt   columnx.Column[model.IndexOutboxEvent, int64]  `dbx:"created_at"`
	UpdatedAt   columnx.Column[model.IndexOutboxEvent, int64]  `dbx:"updated_at"`
}

func newMetadataIndexDocumentsTable() metadataIndexDocumentsTable {
	schema := schemax.MustSchema("metadata_index_documents", metadataIndexDocumentsSchema{})
	return metadataIndexDocumentsTable{
		schema:    schema,
		id:        schema.ID,
		bucket:    schema.Bucket,
		key:       schema.Key,
		versionID: schema.VersionID,
		digest:    schema.Digest,
		state:     schema.State,
		errorText: schema.Error,
		indexedAt: schema.IndexedAt,
		createdAt: schema.CreatedAt,
		updatedAt: schema.UpdatedAt,
	}
}

func newMetadataIndexJobsTable() metadataIndexJobsTable {
	schema := schemax.MustSchema("metadata_index_jobs", metadataIndexJobsSchema{})
	return metadataIndexJobsTable{
		schema:      schema,
		id:          schema.ID,
		kind:        schema.Kind,
		bucket:      schema.Bucket,
		key:         schema.Key,
		versionID:   schema.VersionID,
		status:      schema.Status,
		attempts:    schema.Attempts,
		errorText:   schema.Error,
		availableAt: schema.AvailableAt,
		startedAt:   schema.StartedAt,
		finishedAt:  schema.FinishedAt,
		createdAt:   schema.CreatedAt,
		updatedAt:   schema.UpdatedAt,
	}
}

func newMetadataIndexOutboxTable() metadataIndexOutboxTable {
	schema := schemax.MustSchema("metadata_index_outbox", metadataIndexOutboxSchema{})
	return metadataIndexOutboxTable{
		schema:      schema,
		id:          schema.ID,
		eventType:   schema.EventType,
		bucket:      schema.Bucket,
		key:         schema.Key,
		versionID:   schema.VersionID,
		payload:     schema.Payload,
		status:      schema.Status,
		attempts:    schema.Attempts,
		errorText:   schema.Error,
		availableAt: schema.AvailableAt,
		createdAt:   schema.CreatedAt,
		updatedAt:   schema.UpdatedAt,
	}
}

func (t metadataIndexDocumentsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.id,
		t.bucket,
		t.key,
		t.versionID,
		t.digest,
		t.state,
		t.errorText,
		t.indexedAt,
		t.createdAt,
		t.updatedAt,
	}
}

func (t metadataIndexJobsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.id,
		t.kind,
		t.bucket,
		t.key,
		t.versionID,
		t.status,
		t.attempts,
		t.errorText,
		t.availableAt,
		t.startedAt,
		t.finishedAt,
		t.createdAt,
		t.updatedAt,
	}
}

func (t metadataIndexOutboxTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.id,
		t.eventType,
		t.bucket,
		t.key,
		t.versionID,
		t.payload,
		t.status,
		t.attempts,
		t.errorText,
		t.availableAt,
		t.createdAt,
		t.updatedAt,
	}
}
