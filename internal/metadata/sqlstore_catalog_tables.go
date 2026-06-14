package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
)

var (
	metadataObjectRecords  = newMetadataObjectRecordsTable()
	metadataObjectVersions = newMetadataObjectVersionsTable()
)

type metadataObjectRecordsTable struct {
	table            querydsl.Table
	bucket           columnx.Column[struct{}, string]
	key              columnx.Column[struct{}, string]
	currentVersionID columnx.Column[struct{}, string]
	deleted          columnx.Column[struct{}, int]
	createdAt        columnx.Column[struct{}, int64]
	updatedAt        columnx.Column[struct{}, int64]
}

type metadataObjectVersionsTable struct {
	table              querydsl.Table
	bucket             columnx.Column[struct{}, string]
	key                columnx.Column[struct{}, string]
	versionID          columnx.Column[struct{}, string]
	digest             columnx.Column[struct{}, string]
	etag               columnx.Column[struct{}, string]
	size               columnx.Column[struct{}, int64]
	contentType        columnx.Column[struct{}, string]
	cacheControl       columnx.Column[struct{}, string]
	contentDisposition columnx.Column[struct{}, string]
	contentEncoding    columnx.Column[struct{}, string]
	contentLanguage    columnx.Column[struct{}, string]
	userMetadata       columnx.Column[struct{}, string]
	upstreamID         columnx.Column[struct{}, string]
	upstreamBucket     columnx.Column[struct{}, string]
	upstreamKey        columnx.Column[struct{}, string]
	deleteMarker       columnx.Column[struct{}, int]
	createdAt          columnx.Column[struct{}, int64]
	updatedAt          columnx.Column[struct{}, int64]
}

func newMetadataObjectRecordsTable() metadataObjectRecordsTable {
	table := querydsl.NewTable("metadata_object_records")
	return metadataObjectRecordsTable{
		table:            table,
		bucket:           columnx.Named[string](table, "bucket"),
		key:              columnx.Named[string](table, "object_key"),
		currentVersionID: columnx.Named[string](table, "current_version_id"),
		deleted:          columnx.Named[int](table, "deleted"),
		createdAt:        columnx.Named[int64](table, "created_at"),
		updatedAt:        columnx.Named[int64](table, "updated_at"),
	}
}

func newMetadataObjectVersionsTable() metadataObjectVersionsTable {
	table := querydsl.NewTable("metadata_object_versions")
	return metadataObjectVersionsTable{
		table:              table,
		bucket:             columnx.Named[string](table, "bucket"),
		key:                columnx.Named[string](table, "object_key"),
		versionID:          columnx.Named[string](table, "version_id"),
		digest:             columnx.Named[string](table, "digest"),
		etag:               columnx.Named[string](table, "etag"),
		size:               columnx.Named[int64](table, "size"),
		contentType:        columnx.Named[string](table, "content_type"),
		cacheControl:       columnx.Named[string](table, "cache_control"),
		contentDisposition: columnx.Named[string](table, "content_disposition"),
		contentEncoding:    columnx.Named[string](table, "content_encoding"),
		contentLanguage:    columnx.Named[string](table, "content_language"),
		userMetadata:       columnx.Named[string](table, "user_metadata"),
		upstreamID:         columnx.Named[string](table, "upstream_id"),
		upstreamBucket:     columnx.Named[string](table, "upstream_bucket"),
		upstreamKey:        columnx.Named[string](table, "upstream_key"),
		deleteMarker:       columnx.Named[int](table, "delete_marker"),
		createdAt:          columnx.Named[int64](table, "created_at"),
		updatedAt:          columnx.Named[int64](table, "updated_at"),
	}
}

func (t metadataObjectRecordsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.bucket,
		t.key,
		t.currentVersionID,
		t.deleted,
		t.createdAt,
		t.updatedAt,
	}
}

func (t metadataObjectVersionsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.bucket,
		t.key,
		t.versionID,
		t.digest,
		t.etag,
		t.size,
		t.contentType,
		t.cacheControl,
		t.contentDisposition,
		t.contentEncoding,
		t.contentLanguage,
		t.userMetadata,
		t.upstreamID,
		t.upstreamBucket,
		t.upstreamKey,
		t.deleteMarker,
		t.createdAt,
		t.updatedAt,
	}
}
