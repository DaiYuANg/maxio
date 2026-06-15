package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	schemax "github.com/arcgolabs/dbx/schema"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var (
	metadataObjectRecords       = newMetadataObjectRecordsTable()
	metadataObjectVersions      = newMetadataObjectVersionsTable()
	metadataObjectRecordMapper  = newMetadataEntityMapper[model.ObjectRecord](metadataObjectRecords.schema)
	metadataObjectVersionMapper = newMetadataEntityMapper[model.ObjectVersion](metadataObjectVersions.schema)
)

type metadataObjectRecordsTable struct {
	schema           metadataObjectRecordsSchema
	bucket           columnx.Column[model.ObjectRecord, string]
	key              columnx.Column[model.ObjectRecord, string]
	currentVersionID columnx.Column[model.ObjectRecord, string]
	deleted          columnx.Column[model.ObjectRecord, int]
	createdAt        columnx.Column[model.ObjectRecord, int64]
	updatedAt        columnx.Column[model.ObjectRecord, int64]
}

type metadataObjectVersionsTable struct {
	schema             metadataObjectVersionsSchema
	bucket             columnx.Column[model.ObjectVersion, string]
	key                columnx.Column[model.ObjectVersion, string]
	versionID          columnx.Column[model.ObjectVersion, string]
	digest             columnx.Column[model.ObjectVersion, string]
	etag               columnx.Column[model.ObjectVersion, string]
	size               columnx.Column[model.ObjectVersion, int64]
	contentType        columnx.Column[model.ObjectVersion, string]
	cacheControl       columnx.Column[model.ObjectVersion, string]
	contentDisposition columnx.Column[model.ObjectVersion, string]
	contentEncoding    columnx.Column[model.ObjectVersion, string]
	contentLanguage    columnx.Column[model.ObjectVersion, string]
	userMetadata       columnx.Column[model.ObjectVersion, string]
	upstreamID         columnx.Column[model.ObjectVersion, string]
	upstreamBucket     columnx.Column[model.ObjectVersion, string]
	upstreamKey        columnx.Column[model.ObjectVersion, string]
	deleteMarker       columnx.Column[model.ObjectVersion, int]
	createdAt          columnx.Column[model.ObjectVersion, int64]
	updatedAt          columnx.Column[model.ObjectVersion, int64]
}

type metadataObjectRecordsSchema struct {
	schemax.Schema[model.ObjectRecord]
	Bucket           columnx.Column[model.ObjectRecord, string] `dbx:"bucket"`
	Key              columnx.Column[model.ObjectRecord, string] `dbx:"object_key"`
	CurrentVersionID columnx.Column[model.ObjectRecord, string] `dbx:"current_version_id"`
	Deleted          columnx.Column[model.ObjectRecord, int]    `dbx:"deleted"`
	CreatedAt        columnx.Column[model.ObjectRecord, int64]  `dbx:"created_at"`
	UpdatedAt        columnx.Column[model.ObjectRecord, int64]  `dbx:"updated_at"`
}

type metadataObjectVersionsSchema struct {
	schemax.Schema[model.ObjectVersion]
	Bucket             columnx.Column[model.ObjectVersion, string] `dbx:"bucket"`
	Key                columnx.Column[model.ObjectVersion, string] `dbx:"object_key"`
	VersionID          columnx.Column[model.ObjectVersion, string] `dbx:"version_id"`
	Digest             columnx.Column[model.ObjectVersion, string] `dbx:"digest"`
	ETag               columnx.Column[model.ObjectVersion, string] `dbx:"etag"`
	Size               columnx.Column[model.ObjectVersion, int64]  `dbx:"size"`
	ContentType        columnx.Column[model.ObjectVersion, string] `dbx:"content_type"`
	CacheControl       columnx.Column[model.ObjectVersion, string] `dbx:"cache_control"`
	ContentDisposition columnx.Column[model.ObjectVersion, string] `dbx:"content_disposition"`
	ContentEncoding    columnx.Column[model.ObjectVersion, string] `dbx:"content_encoding"`
	ContentLanguage    columnx.Column[model.ObjectVersion, string] `dbx:"content_language"`
	UserMetadata       columnx.Column[model.ObjectVersion, string] `dbx:"user_metadata"`
	UpstreamID         columnx.Column[model.ObjectVersion, string] `dbx:"upstream_id"`
	UpstreamBucket     columnx.Column[model.ObjectVersion, string] `dbx:"upstream_bucket"`
	UpstreamKey        columnx.Column[model.ObjectVersion, string] `dbx:"upstream_key"`
	DeleteMarker       columnx.Column[model.ObjectVersion, int]    `dbx:"delete_marker"`
	CreatedAt          columnx.Column[model.ObjectVersion, int64]  `dbx:"created_at"`
	UpdatedAt          columnx.Column[model.ObjectVersion, int64]  `dbx:"updated_at"`
}

func newMetadataObjectRecordsTable() metadataObjectRecordsTable {
	schema := schemax.MustSchema("metadata_object_records", metadataObjectRecordsSchema{})
	return metadataObjectRecordsTable{
		schema:           schema,
		bucket:           schema.Bucket,
		key:              schema.Key,
		currentVersionID: schema.CurrentVersionID,
		deleted:          schema.Deleted,
		createdAt:        schema.CreatedAt,
		updatedAt:        schema.UpdatedAt,
	}
}

func newMetadataObjectVersionsTable() metadataObjectVersionsTable {
	schema := schemax.MustSchema("metadata_object_versions", metadataObjectVersionsSchema{})
	return metadataObjectVersionsTable{
		schema:             schema,
		bucket:             schema.Bucket,
		key:                schema.Key,
		versionID:          schema.VersionID,
		digest:             schema.Digest,
		etag:               schema.ETag,
		size:               schema.Size,
		contentType:        schema.ContentType,
		cacheControl:       schema.CacheControl,
		contentDisposition: schema.ContentDisposition,
		contentEncoding:    schema.ContentEncoding,
		contentLanguage:    schema.ContentLanguage,
		userMetadata:       schema.UserMetadata,
		upstreamID:         schema.UpstreamID,
		upstreamBucket:     schema.UpstreamBucket,
		upstreamKey:        schema.UpstreamKey,
		deleteMarker:       schema.DeleteMarker,
		createdAt:          schema.CreatedAt,
		updatedAt:          schema.UpdatedAt,
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
