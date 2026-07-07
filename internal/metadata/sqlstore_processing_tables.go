package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	schemax "github.com/arcgolabs/dbx/schema"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var metadataProcessingRecords = newMetadataProcessingRecordsTable()

type metadataProcessingRecordsTable struct {
	schema    metadataProcessingRecordsSchema
	id        columnx.Column[model.ProcessingRecord, string]
	bucket    columnx.Column[model.ProcessingRecord, string]
	key       columnx.Column[model.ProcessingRecord, string]
	versionID columnx.Column[model.ProcessingRecord, string]
	digest    columnx.Column[model.ProcessingRecord, string]
	mode      columnx.Column[model.ProcessingRecord, string]
	status    columnx.Column[model.ProcessingRecord, string]
	errorText columnx.Column[model.ProcessingRecord, string]
	results   columnx.Column[model.ProcessingRecord, string]
	createdAt columnx.Column[model.ProcessingRecord, int64]
	updatedAt columnx.Column[model.ProcessingRecord, int64]
}

type metadataProcessingRecordsSchema struct {
	schemax.Schema[model.ProcessingRecord]
	ID        columnx.Column[model.ProcessingRecord, string] `dbx:"record_id,pk"`
	Bucket    columnx.Column[model.ProcessingRecord, string] `dbx:"bucket"`
	Key       columnx.Column[model.ProcessingRecord, string] `dbx:"object_key"`
	VersionID columnx.Column[model.ProcessingRecord, string] `dbx:"version_id"`
	Digest    columnx.Column[model.ProcessingRecord, string] `dbx:"digest"`
	Mode      columnx.Column[model.ProcessingRecord, string] `dbx:"mode"`
	Status    columnx.Column[model.ProcessingRecord, string] `dbx:"status"`
	Error     columnx.Column[model.ProcessingRecord, string] `dbx:"error"`
	Results   columnx.Column[model.ProcessingRecord, string] `dbx:"results"`
	CreatedAt columnx.Column[model.ProcessingRecord, int64]  `dbx:"created_at"`
	UpdatedAt columnx.Column[model.ProcessingRecord, int64]  `dbx:"updated_at"`
}

func newMetadataProcessingRecordsTable() metadataProcessingRecordsTable {
	schema := schemax.MustSchema("metadata_processing_records", metadataProcessingRecordsSchema{})
	return metadataProcessingRecordsTable{
		schema:    schema,
		id:        schema.ID,
		bucket:    schema.Bucket,
		key:       schema.Key,
		versionID: schema.VersionID,
		digest:    schema.Digest,
		mode:      schema.Mode,
		status:    schema.Status,
		errorText: schema.Error,
		results:   schema.Results,
		createdAt: schema.CreatedAt,
		updatedAt: schema.UpdatedAt,
	}
}
