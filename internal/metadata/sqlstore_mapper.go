package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx"
	dbxmapper "github.com/arcgolabs/dbx/mapper"
	"github.com/arcgolabs/dbx/querydsl"
	schemax "github.com/arcgolabs/dbx/schema"
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

func newMetadataEntityMapper[T any](schema schemax.Resource) dbxmapper.Mapper[T] {
	return dbxmapper.MustMapperWithOptions[T](
		schema,
		dbxmapper.WithMapperCodecs(metadataBoolIntCodec),
	)
}

func newMetadataProjectionMapper[T any]() dbxmapper.StructMapper[T] {
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
	item, err := dbx.QueryOption(ensureContext(ctx), session, query, rowMapper)
	if err != nil {
		return zero, false, fmt.Errorf("query %s: %w", label, err)
	}
	value, found := item.Get()
	return value, found, nil
}

func querySQLScalarOption[T any](
	ctx context.Context,
	store *SQLMetadata,
	query querydsl.SelectResult[T],
	label string,
) (T, bool, error) {
	var zero T
	session, err := metadataDBSession(store)
	if err != nil {
		return zero, false, fmt.Errorf("query %s: %w", label, err)
	}
	item, err := dbx.QueryScalarOption(ensureContext(ctx), session, query)
	if err != nil {
		return zero, false, fmt.Errorf("query %s: %w", label, err)
	}
	value, found := item.Get()
	return value, found, nil
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

func querySQLScalarsInTx[T any](
	ctx context.Context,
	store *SQLMetadata,
	tx *sql.Tx,
	query querydsl.SelectResult[T],
	label string,
) (*collectionlist.List[T], error) {
	return querySQLScalarsLimitInTx(ctx, store, tx, query, label, 0)
}

func querySQLScalarOptionInTx[T any](
	ctx context.Context,
	store *SQLMetadata,
	tx *sql.Tx,
	query querydsl.SelectResult[T],
	label string,
) (T, bool, error) {
	var zero T
	items, err := querySQLScalarsLimitInTx(ctx, store, tx, query, label, 2)
	if err != nil {
		return zero, false, err
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

func querySQLScalarsLimitInTx[T any](
	ctx context.Context,
	store *SQLMetadata,
	tx *sql.Tx,
	query querydsl.SelectResult[T],
	label string,
	limit int,
) (*collectionlist.List[T], error) {
	rows, err := store.txQueryBuilderContext(ctx, tx, query)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}
	defer closeSQLRows(store, rows, label)

	items, err := scanSQLScalarRowsLimit[T](rows, limit)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", label, err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", label, err)
	}
	return items, nil
}

func scanSQLScalarRowsLimit[T any](rows *sql.Rows, limit int) (*collectionlist.List[T], error) {
	items := collectionlist.NewList[T]()
	for rows.Next() {
		var item T
		if err := rows.Scan(&item); err != nil {
			return nil, fmt.Errorf("scan sql scalar row: %w", err)
		}
		items.Add(item)
		if limit > 0 && items.Len() >= limit {
			break
		}
	}
	return items, nil
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
