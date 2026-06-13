package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
)

var metadataObjects = newMetadataObjectsTable()

type metadataObjectsTable struct {
	table                querydsl.Table
	bucket               columnx.Column[struct{}, string]
	key                  columnx.Column[struct{}, string]
	hash                 columnx.Column[struct{}, string]
	etag                 columnx.Column[struct{}, string]
	size                 columnx.Column[struct{}, int64]
	contentType          columnx.Column[struct{}, string]
	cacheControl         columnx.Column[struct{}, string]
	contentDisposition   columnx.Column[struct{}, string]
	contentEncoding      columnx.Column[struct{}, string]
	contentLanguage      columnx.Column[struct{}, string]
	userMetadata         columnx.Column[struct{}, string]
	updatedAt            columnx.Column[struct{}, int64]
	state                columnx.Column[struct{}, string]
	writeIntentID        columnx.Column[struct{}, any]
	writeIntentStage     columnx.Column[struct{}, any]
	writeIntentStartedAt columnx.Column[struct{}, any]
	writeIntentUpdatedAt columnx.Column[struct{}, any]
	shardPlacements      columnx.Column[struct{}, string]
	shardChecksums       columnx.Column[struct{}, string]
	shardSizes           columnx.Column[struct{}, string]
}

func newMetadataObjectsTable() metadataObjectsTable {
	table := querydsl.NewTable("metadata_objects")
	return metadataObjectsTable{
		table:                table,
		bucket:               columnx.Named[string](table, "bucket"),
		key:                  columnx.Named[string](table, "object_key"),
		hash:                 columnx.Named[string](table, "hash"),
		etag:                 columnx.Named[string](table, "etag"),
		size:                 columnx.Named[int64](table, "size"),
		contentType:          columnx.Named[string](table, "content_type"),
		cacheControl:         columnx.Named[string](table, "cache_control"),
		contentDisposition:   columnx.Named[string](table, "content_disposition"),
		contentEncoding:      columnx.Named[string](table, "content_encoding"),
		contentLanguage:      columnx.Named[string](table, "content_language"),
		userMetadata:         columnx.Named[string](table, "user_metadata"),
		updatedAt:            columnx.Named[int64](table, "updated_at"),
		state:                columnx.Named[string](table, "state"),
		writeIntentID:        columnx.Named[any](table, "write_intent_id"),
		writeIntentStage:     columnx.Named[any](table, "write_intent_stage"),
		writeIntentStartedAt: columnx.Named[any](table, "write_intent_started_at"),
		writeIntentUpdatedAt: columnx.Named[any](table, "write_intent_updated_at"),
		shardPlacements:      columnx.Named[string](table, "shard_placements"),
		shardChecksums:       columnx.Named[string](table, "shard_checksums"),
		shardSizes:           columnx.Named[string](table, "shard_sizes"),
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
		t.shardPlacements,
		t.shardChecksums,
		t.shardSizes,
	}
}
