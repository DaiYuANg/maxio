package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx"
	codecx "github.com/arcgolabs/dbx/codec"
	dbxmapper "github.com/arcgolabs/dbx/mapper"
	"github.com/arcgolabs/dbx/querydsl"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var (
	metadataBoolIntCodec = codecx.New[bool]("bool_int", decodeBoolInt, encodeBoolInt)

	metadataBucketMapper           = newMetadataStructMapper[model.Bucket]()
	metadataNameMapper             = newMetadataStructMapper[metadataNameRow]()
	metadataUpstreamMapper         = newMetadataStructMapper[model.Upstream]()
	metadataObjectMetaMapper       = newMetadataStructMapper[objectMetaRow]()
	metadataObjectRecordMapper     = newMetadataStructMapper[model.ObjectRecord]()
	metadataObjectVersionMapper    = newMetadataStructMapper[model.ObjectVersion]()
	metadataDigestRefMapper        = newMetadataStructMapper[model.DigestRef]()
	metadataBlobRefMapper          = newMetadataStructMapper[BlobRef]()
	metadataBlobRefCounterMapper   = newMetadataStructMapper[blobRefCounterRow]()
	metadataHashMapper             = newMetadataStructMapper[metadataHashRow]()
	metadataIndexDocumentMapper    = newMetadataStructMapper[model.IndexDocument]()
	metadataIndexJobMapper         = newMetadataStructMapper[model.IndexJob]()
	metadataIndexOutboxEventMapper = newMetadataStructMapper[model.IndexOutboxEvent]()
)

type metadataNameRow struct {
	Name string `dbx:"name"`
}

type metadataHashRow struct {
	Hash string `dbx:"hash"`
}

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

func newMetadataStructMapper[T any]() dbxmapper.StructMapper[T] {
	return dbxmapper.MustStructMapperWithOptions[T](
		dbxmapper.WithMapperCodecs(metadataBoolIntCodec),
	)
}

func querySQLRows[E any](
	ctx context.Context,
	store *SQLMetadata,
	query querydsl.Builder,
	label string,
	rowMapper dbxmapper.RowsScanner[E],
) (*collectionlist.List[E], error) {
	session, err := metadataDBSession(store)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}
	items, err := dbx.QueryAll(ensureContext(ctx), session, query, rowMapper)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}
	return items, nil
}

func querySQLOne[E any](
	ctx context.Context,
	store *SQLMetadata,
	query querydsl.Builder,
	label string,
	rowMapper dbxmapper.RowsScanner[E],
) (E, bool, error) {
	var zero E
	session, err := metadataDBSession(store)
	if err != nil {
		return zero, false, fmt.Errorf("query %s: %w", label, err)
	}
	item, err := dbx.QueryOne(ensureContext(ctx), session, query, rowMapper)
	if errors.Is(err, sql.ErrNoRows) {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, fmt.Errorf("query %s: %w", label, err)
	}
	return item, true, nil
}

func querySQLRowsInTx[E any](
	ctx context.Context,
	store *SQLMetadata,
	tx *sql.Tx,
	query querydsl.Builder,
	label string,
	rowMapper dbxmapper.RowsScanner[E],
) (*collectionlist.List[E], error) {
	rows, err := store.txQueryBuilderContext(ctx, tx, query)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}
	defer closeSQLRows(store, rows, label)

	items, err := rowMapper.ScanRows(rows)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", label, err)
	}
	return items, nil
}

func querySQLOneInTx[E any](
	ctx context.Context,
	store *SQLMetadata,
	tx *sql.Tx,
	query querydsl.Builder,
	label string,
	rowMapper dbxmapper.RowsScanner[E],
) (E, bool, error) {
	var zero E
	rows, err := store.txQueryBuilderContext(ctx, tx, query)
	if err != nil {
		return zero, false, fmt.Errorf("query %s: %w", label, err)
	}
	defer closeSQLRows(store, rows, label)

	items, err := mapSQLRowsLimit(ctx, rows, rowMapper, 2)
	if err != nil {
		return zero, false, fmt.Errorf("query %s: %w", label, err)
	}
	if err := rows.Err(); err != nil {
		return zero, false, fmt.Errorf("iterate %s: %w", label, err)
	}
	switch items.Len() {
	case 0:
		return zero, false, nil
	case 1:
		item, _ := items.Get(0)
		return item, true, nil
	default:
		return zero, false, fmt.Errorf("query %s: expected one row, got %d", label, items.Len())
	}
}

func mapSQLRowsLimit[E any](
	ctx context.Context,
	rows *sql.Rows,
	rowMapper dbxmapper.RowsScanner[E],
	limit int,
) (*collectionlist.List[E], error) {
	if limitScanner, ok := rowMapper.(dbxmapper.LimitScanner[E]); ok {
		items, err := limitScanner.ScanRowsLimit(ensureContext(ctx), rows, limit)
		if err != nil {
			return nil, fmt.Errorf("map sql rows limit: %w", err)
		}
		return items, nil
	}
	items, err := rowMapper.ScanRows(rows)
	if err != nil {
		return nil, fmt.Errorf("map sql rows: %w", err)
	}
	return items, nil
}

func metadataDBSession(store *SQLMetadata) (dbx.Session, error) {
	if store == nil || store.dbxDB == nil {
		return nil, errors.New("metadata dbx session is nil")
	}
	return store.dbxDB, nil
}

func closeSQLRows(store *SQLMetadata, rows *sql.Rows, label string) {
	if rows == nil {
		return
	}
	if closeErr := rows.Close(); closeErr != nil && store != nil && store.logger != nil {
		store.logger.Error("close sql metadata rows", "rows", label, "error", closeErr)
	}
}

func objectMetaRowsToList(rows *collectionlist.List[objectMetaRow]) *collectionlist.List[model.ObjectMeta] {
	if rows == nil {
		return collectionlist.NewList[model.ObjectMeta]()
	}
	return collectionlist.MapList(rows, func(_ int, row objectMetaRow) model.ObjectMeta {
		return row.objectMeta()
	})
}

func metadataHashRowsToList(rows *collectionlist.List[metadataHashRow]) *collectionlist.List[string] {
	if rows == nil {
		return collectionlist.NewList[string]()
	}
	return collectionlist.MapList(rows, func(_ int, row metadataHashRow) string {
		return row.Hash
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

func decodeBoolInt(src any) (bool, error) {
	switch value := src.(type) {
	case nil:
		return false, nil
	case bool:
		return value, nil
	case []byte:
		return parseBoolInt(string(value))
	case sql.RawBytes:
		return parseBoolInt(string(value))
	case string:
		return parseBoolInt(value)
	default:
		return decodeNumericBoolInt(src)
	}
}

func decodeNumericBoolInt(src any) (bool, error) {
	value := reflect.ValueOf(src)
	if value.CanInt() {
		return value.Int() != 0, nil
	}
	if value.CanUint() {
		return value.Uint() != 0, nil
	}
	return false, fmt.Errorf("unsupported bool_int source %T", src)
}

func encodeBoolInt(value bool) (any, error) {
	if value {
		return 1, nil
	}
	return 0, nil
}

func parseBoolInt(raw string) (bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, nil
	}
	if value, err := strconv.ParseBool(trimmed); err == nil {
		return value, nil
	}
	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return false, fmt.Errorf("parse bool_int: %w", err)
	}
	return value != 0, nil
}
