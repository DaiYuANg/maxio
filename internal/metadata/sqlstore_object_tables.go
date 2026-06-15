package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	schemax "github.com/arcgolabs/dbx/schema"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var metadataObjects = newMetadataObjectsTable()

type metadataObjectsTable struct {
	schema               metadataObjectsSchema
	bucket               columnx.Column[model.ObjectMeta, string]
	key                  columnx.Column[model.ObjectMeta, string]
	hash                 columnx.Column[model.ObjectMeta, string]
	etag                 columnx.Column[model.ObjectMeta, string]
	size                 columnx.Column[model.ObjectMeta, int64]
	contentType          columnx.Column[model.ObjectMeta, string]
	cacheControl         columnx.Column[model.ObjectMeta, string]
	contentDisposition   columnx.Column[model.ObjectMeta, string]
	contentEncoding      columnx.Column[model.ObjectMeta, string]
	contentLanguage      columnx.Column[model.ObjectMeta, string]
	userMetadata         columnx.Column[model.ObjectMeta, string]
	updatedAt            columnx.Column[model.ObjectMeta, int64]
	state                columnx.Column[model.ObjectMeta, string]
	writeIntentID        columnx.Column[model.ObjectMeta, any]
	writeIntentStage     columnx.Column[model.ObjectMeta, any]
	writeIntentStartedAt columnx.Column[model.ObjectMeta, any]
	writeIntentUpdatedAt columnx.Column[model.ObjectMeta, any]
}

type metadataObjectsSchema struct {
	schemax.Schema[model.ObjectMeta]
	Bucket               columnx.Column[model.ObjectMeta, string] `dbx:"bucket"`
	Key                  columnx.Column[model.ObjectMeta, string] `dbx:"object_key"`
	Hash                 columnx.Column[model.ObjectMeta, string] `dbx:"hash"`
	ETag                 columnx.Column[model.ObjectMeta, string] `dbx:"etag"`
	Size                 columnx.Column[model.ObjectMeta, int64]  `dbx:"size"`
	ContentType          columnx.Column[model.ObjectMeta, string] `dbx:"content_type"`
	CacheControl         columnx.Column[model.ObjectMeta, string] `dbx:"cache_control"`
	ContentDisposition   columnx.Column[model.ObjectMeta, string] `dbx:"content_disposition"`
	ContentEncoding      columnx.Column[model.ObjectMeta, string] `dbx:"content_encoding"`
	ContentLanguage      columnx.Column[model.ObjectMeta, string] `dbx:"content_language"`
	UserMetadata         columnx.Column[model.ObjectMeta, string] `dbx:"user_metadata"`
	UpdatedAt            columnx.Column[model.ObjectMeta, int64]  `dbx:"updated_at"`
	State                columnx.Column[model.ObjectMeta, string] `dbx:"state"`
	WriteIntentID        columnx.Column[model.ObjectMeta, any]    `dbx:"write_intent_id"`
	WriteIntentStage     columnx.Column[model.ObjectMeta, any]    `dbx:"write_intent_stage"`
	WriteIntentStartedAt columnx.Column[model.ObjectMeta, any]    `dbx:"write_intent_started_at"`
	WriteIntentUpdatedAt columnx.Column[model.ObjectMeta, any]    `dbx:"write_intent_updated_at"`
}

func newMetadataObjectsTable() metadataObjectsTable {
	schema := schemax.MustSchema("metadata_objects", metadataObjectsSchema{})
	return metadataObjectsTable{
		schema:               schema,
		bucket:               schema.Bucket,
		key:                  schema.Key,
		hash:                 schema.Hash,
		etag:                 schema.ETag,
		size:                 schema.Size,
		contentType:          schema.ContentType,
		cacheControl:         schema.CacheControl,
		contentDisposition:   schema.ContentDisposition,
		contentEncoding:      schema.ContentEncoding,
		contentLanguage:      schema.ContentLanguage,
		userMetadata:         schema.UserMetadata,
		updatedAt:            schema.UpdatedAt,
		state:                schema.State,
		writeIntentID:        schema.WriteIntentID,
		writeIntentStage:     schema.WriteIntentStage,
		writeIntentStartedAt: schema.WriteIntentStartedAt,
		writeIntentUpdatedAt: schema.WriteIntentUpdatedAt,
	}
}

func (t metadataObjectsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.bucket,
		t.key,
		t.hash,
		t.etag,
		t.size,
		t.contentType,
		t.cacheControl,
		t.contentDisposition,
		t.contentEncoding,
		t.contentLanguage,
		t.userMetadata,
		t.updatedAt,
		t.state,
		t.writeIntentID,
		t.writeIntentStage,
		t.writeIntentStartedAt,
		t.writeIntentUpdatedAt,
	}
}
