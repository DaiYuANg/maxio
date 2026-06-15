package metadata

import (
	"database/sql"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	dbxmapper "github.com/arcgolabs/dbx/mapper"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var (
	metadataObjectMetaMapper     = newMetadataProjectionMapper[objectMetaRow]()
	metadataBlobRefCounterMapper = newMetadataProjectionMapper[blobRefCounterRow]()
)

type blobRefCounterRow struct {
	Path     string `dbx:"path"`
	RefCount int    `dbx:"ref_count"`
}

type objectMetaRow struct {
	model.ObjectMeta `dbx:",inline"`

	WriteIntentID        sql.NullString `dbx:"write_intent_id"`
	WriteIntentStage     sql.NullString `dbx:"write_intent_stage"`
	WriteIntentStartedAt time.Time      `dbx:"write_intent_started_at,codec=unix_nano_time"`
	WriteIntentUpdatedAt time.Time      `dbx:"write_intent_updated_at,codec=unix_nano_time"`
}

func newMetadataProjectionMapper[T any]() dbxmapper.StructMapper[T] {
	return dbxmapper.MustStructMapperWithOptions[T](
		dbxmapper.WithMapperCodecs(metadataBoolIntCodec),
	)
}

func objectMetaRowsToList(rows *collectionlist.List[objectMetaRow]) *collectionlist.List[model.ObjectMeta] {
	if rows == nil {
		return collectionlist.NewList[model.ObjectMeta]()
	}
	return collectionlist.MapList(rows, func(_ int, row objectMetaRow) model.ObjectMeta {
		return row.objectMeta()
	})
}

func (row objectMetaRow) objectMeta() model.ObjectMeta {
	meta := row.ObjectMeta
	if row.WriteIntentID.Valid {
		meta.WriteIntent = &model.WriteIntent{
			ID:        row.WriteIntentID.String,
			Stage:     emptyStringOrDefault(row.WriteIntentStage),
			StartedAt: row.WriteIntentStartedAt,
			UpdatedAt: row.WriteIntentUpdatedAt,
		}
	}
	return meta
}
