package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
)

var (
	metadataIndexDocuments = newMetadataIndexDocumentsTable()
	metadataIndexJobs      = newMetadataIndexJobsTable()
	metadataIndexOutbox    = newMetadataIndexOutboxTable()
)

type metadataIndexDocumentsTable struct {
	table     querydsl.Table
	id        columnx.Column[struct{}, string]
	bucket    columnx.Column[struct{}, string]
	key       columnx.Column[struct{}, string]
	versionID columnx.Column[struct{}, string]
	digest    columnx.Column[struct{}, string]
	state     columnx.Column[struct{}, string]
	errorText columnx.Column[struct{}, string]
	indexedAt columnx.Column[struct{}, any]
	createdAt columnx.Column[struct{}, int64]
	updatedAt columnx.Column[struct{}, int64]
}

type metadataIndexJobsTable struct {
	table       querydsl.Table
	id          columnx.Column[struct{}, string]
	kind        columnx.Column[struct{}, string]
	bucket      columnx.Column[struct{}, string]
	key         columnx.Column[struct{}, string]
	versionID   columnx.Column[struct{}, string]
	status      columnx.Column[struct{}, string]
	attempts    columnx.Column[struct{}, int]
	errorText   columnx.Column[struct{}, string]
	availableAt columnx.Column[struct{}, int64]
	startedAt   columnx.Column[struct{}, any]
	finishedAt  columnx.Column[struct{}, any]
	createdAt   columnx.Column[struct{}, int64]
	updatedAt   columnx.Column[struct{}, int64]
}

type metadataIndexOutboxTable struct {
	table       querydsl.Table
	id          columnx.Column[struct{}, string]
	eventType   columnx.Column[struct{}, string]
	bucket      columnx.Column[struct{}, string]
	key         columnx.Column[struct{}, string]
	versionID   columnx.Column[struct{}, string]
	payload     columnx.Column[struct{}, string]
	status      columnx.Column[struct{}, string]
	attempts    columnx.Column[struct{}, int]
	errorText   columnx.Column[struct{}, string]
	availableAt columnx.Column[struct{}, int64]
	createdAt   columnx.Column[struct{}, int64]
	updatedAt   columnx.Column[struct{}, int64]
}

func newMetadataIndexDocumentsTable() metadataIndexDocumentsTable {
	table := querydsl.NewTable("metadata_index_documents")
	return metadataIndexDocumentsTable{
		table:     table,
		id:        columnx.Named[string](table, "document_id"),
		bucket:    columnx.Named[string](table, "bucket"),
		key:       columnx.Named[string](table, "object_key"),
		versionID: columnx.Named[string](table, "version_id"),
		digest:    columnx.Named[string](table, "digest"),
		state:     columnx.Named[string](table, "state"),
		errorText: columnx.Named[string](table, "error"),
		indexedAt: columnx.Named[any](table, "indexed_at"),
		createdAt: columnx.Named[int64](table, "created_at"),
		updatedAt: columnx.Named[int64](table, "updated_at"),
	}
}

func newMetadataIndexJobsTable() metadataIndexJobsTable {
	table := querydsl.NewTable("metadata_index_jobs")
	return metadataIndexJobsTable{
		table:       table,
		id:          columnx.Named[string](table, "job_id"),
		kind:        columnx.Named[string](table, "kind"),
		bucket:      columnx.Named[string](table, "bucket"),
		key:         columnx.Named[string](table, "object_key"),
		versionID:   columnx.Named[string](table, "version_id"),
		status:      columnx.Named[string](table, "status"),
		attempts:    columnx.Named[int](table, "attempts"),
		errorText:   columnx.Named[string](table, "error"),
		availableAt: columnx.Named[int64](table, "available_at"),
		startedAt:   columnx.Named[any](table, "started_at"),
		finishedAt:  columnx.Named[any](table, "finished_at"),
		createdAt:   columnx.Named[int64](table, "created_at"),
		updatedAt:   columnx.Named[int64](table, "updated_at"),
	}
}

func newMetadataIndexOutboxTable() metadataIndexOutboxTable {
	table := querydsl.NewTable("metadata_index_outbox")
	return metadataIndexOutboxTable{
		table:       table,
		id:          columnx.Named[string](table, "event_id"),
		eventType:   columnx.Named[string](table, "event_type"),
		bucket:      columnx.Named[string](table, "bucket"),
		key:         columnx.Named[string](table, "object_key"),
		versionID:   columnx.Named[string](table, "version_id"),
		payload:     columnx.Named[string](table, "payload"),
		status:      columnx.Named[string](table, "status"),
		attempts:    columnx.Named[int](table, "attempts"),
		errorText:   columnx.Named[string](table, "error"),
		availableAt: columnx.Named[int64](table, "available_at"),
		createdAt:   columnx.Named[int64](table, "created_at"),
		updatedAt:   columnx.Named[int64](table, "updated_at"),
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
