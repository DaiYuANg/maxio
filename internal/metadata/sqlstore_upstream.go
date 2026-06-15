package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	collectionset "github.com/arcgolabs/collectionx/set"
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var metadataUpstreams = newMetadataUpstreamsTable()

type metadataUpstreamsTable struct {
	table     querydsl.Table
	id        columnx.Column[struct{}, string]
	name      columnx.Column[struct{}, string]
	endpoint  columnx.Column[struct{}, string]
	region    columnx.Column[struct{}, string]
	weight    columnx.Column[struct{}, int]
	priority  columnx.Column[struct{}, int]
	buckets   columnx.Column[struct{}, string]
	enabled   columnx.Column[struct{}, int]
	createdAt columnx.Column[struct{}, int64]
	updatedAt columnx.Column[struct{}, int64]
}

func newMetadataUpstreamsTable() metadataUpstreamsTable {
	table := querydsl.NewTable("metadata_upstreams")
	return metadataUpstreamsTable{
		table:     table,
		id:        columnx.Named[string](table, "id"),
		name:      columnx.Named[string](table, "name"),
		endpoint:  columnx.Named[string](table, "endpoint"),
		region:    columnx.Named[string](table, "region"),
		weight:    columnx.Named[int](table, "weight"),
		priority:  columnx.Named[int](table, "priority"),
		buckets:   columnx.Named[string](table, "buckets"),
		enabled:   columnx.Named[int](table, "enabled"),
		createdAt: columnx.Named[int64](table, "created_at"),
		updatedAt: columnx.Named[int64](table, "updated_at"),
	}
}

func (t metadataUpstreamsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.id,
		t.name,
		t.endpoint,
		t.region,
		t.weight,
		t.priority,
		t.buckets,
		t.enabled,
		t.createdAt,
		t.updatedAt,
	}
}

func (s *SQLMetadata) ListUpstreams(ctx context.Context) ([]model.Upstream, error) {
	query := querydsl.SelectFrom(metadataUpstreams.table, metadataUpstreams.selectItems()...).
		OrderBy(metadataUpstreams.priority.Asc(), metadataUpstreams.name.Asc(), metadataUpstreams.id.Asc())
	upstreams, err := listSQLRows(ctx, s, query, "upstreams", scanUpstream)
	if err != nil {
		return nil, err
	}
	return upstreams.Values(), nil
}

func (s *SQLMetadata) GetUpstream(ctx context.Context, id string) (model.Upstream, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.Upstream{}, false, ErrBadRequest
	}

	query := querydsl.SelectFrom(metadataUpstreams.table, metadataUpstreams.selectItems()...).
		Where(metadataUpstreams.id.Eq(id)).
		Limit(1)
	row, err := s.queryRowBuilderContext(ctx, query)
	if err != nil {
		return model.Upstream{}, false, fmt.Errorf("query upstream: %w", err)
	}
	upstream, err := scanUpstream(row)
	if err != nil {
		if errorsIsNoRows(err) {
			return model.Upstream{}, false, nil
		}
		return model.Upstream{}, false, fmt.Errorf("query upstream: %w", err)
	}
	return upstream, true, nil
}

func (s *SQLMetadata) UpsertUpstream(ctx context.Context, upstream model.Upstream) (model.Upstream, error) {
	upstream, err := normalizeUpstream(upstream)
	if err != nil {
		return model.Upstream{}, err
	}

	now := time.Now().UTC()
	if upstream.CreatedAt.IsZero() {
		upstream.CreatedAt = now
	}
	upstream.UpdatedAt = now

	query := querydsl.InsertInto(metadataUpstreams.table).
		Values(
			metadataUpstreams.id.Set(upstream.ID),
			metadataUpstreams.name.Set(upstream.Name),
			metadataUpstreams.endpoint.Set(upstream.Endpoint),
			metadataUpstreams.region.Set(upstream.Region),
			metadataUpstreams.weight.Set(upstream.Weight),
			metadataUpstreams.priority.Set(upstream.Priority),
			metadataUpstreams.buckets.Set(marshalStrings(upstream.Buckets)),
			metadataUpstreams.enabled.Set(boolToInt(upstream.Enabled)),
			metadataUpstreams.createdAt.Set(upstream.CreatedAt.UnixNano()),
			metadataUpstreams.updatedAt.Set(upstream.UpdatedAt.UnixNano()),
		).
		OnConflict(metadataUpstreams.id).
		DoUpdateSet(
			metadataUpstreams.name.SetExcluded(),
			metadataUpstreams.endpoint.SetExcluded(),
			metadataUpstreams.region.SetExcluded(),
			metadataUpstreams.weight.SetExcluded(),
			metadataUpstreams.priority.SetExcluded(),
			metadataUpstreams.buckets.SetExcluded(),
			metadataUpstreams.enabled.SetExcluded(),
			metadataUpstreams.updatedAt.SetExcluded(),
		)

	_, execErr := s.execBuilderContext(ctx, query)
	if execErr != nil {
		return model.Upstream{}, fmt.Errorf("upsert upstream: %w", execErr)
	}
	stored, _, err := s.GetUpstream(ctx, upstream.ID)
	if err != nil {
		return model.Upstream{}, err
	}
	return stored, nil
}

func (s *SQLMetadata) DeleteUpstream(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}

	query := querydsl.DeleteFrom(metadataUpstreams.table).Where(metadataUpstreams.id.Eq(id))
	result, err := s.execBuilderContext(ctx, query)
	if err != nil {
		return false, fmt.Errorf("delete upstream: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete upstream rows: %w", err)
	}
	return affected > 0, nil
}

func scanUpstream(scanner interface{ Scan(dest ...any) error }) (model.Upstream, error) {
	var (
		upstream model.Upstream
		buckets  sql.NullString
		enabled  int
		created  int64
		updated  int64
	)
	if err := scanner.Scan(
		&upstream.ID,
		&upstream.Name,
		&upstream.Endpoint,
		&upstream.Region,
		&upstream.Weight,
		&upstream.Priority,
		&buckets,
		&enabled,
		&created,
		&updated,
	); err != nil {
		return model.Upstream{}, fmt.Errorf("scan upstream: %w", err)
	}
	if err := decodeJSON(buckets, &upstream.Buckets); err != nil {
		return model.Upstream{}, fmt.Errorf("decode upstream buckets: %w", err)
	}
	upstream.Enabled = enabled != 0
	upstream.CreatedAt = unixNanoToTime(created)
	upstream.UpdatedAt = unixNanoToTime(updated)
	return upstream, nil
}

func normalizeUpstream(upstream model.Upstream) (model.Upstream, error) {
	upstream.ID = strings.TrimSpace(upstream.ID)
	upstream.Name = strings.TrimSpace(upstream.Name)
	upstream.Endpoint = strings.TrimSpace(upstream.Endpoint)
	upstream.Region = strings.TrimSpace(upstream.Region)
	if upstream.ID == "" {
		upstream.ID = upstream.Name
	}
	if upstream.Name == "" {
		upstream.Name = upstream.ID
	}
	if upstream.ID == "" || upstream.Name == "" || upstream.Endpoint == "" {
		return model.Upstream{}, ErrBadRequest
	}
	upstream.Buckets = normalizeStringList(upstream.Buckets)
	return upstream, nil
}

func normalizeStringList(values []string) []string {
	uniqValues := collectionset.NewOrderedSetWithCapacity[string](len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		uniqValues.Add(trimmed)
	}
	return uniqValues.Values()
}
