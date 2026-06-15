package metadata

import (
	"context"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	schemax "github.com/arcgolabs/dbx/schema"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var (
	metadataUpstreams = newMetadataUpstreamsTable()
)

type metadataUpstreamsTable struct {
	schema    metadataUpstreamsSchema
	id        columnx.Column[model.Upstream, string]
	name      columnx.Column[model.Upstream, string]
	endpoint  columnx.Column[model.Upstream, string]
	region    columnx.Column[model.Upstream, string]
	weight    columnx.Column[model.Upstream, int]
	priority  columnx.Column[model.Upstream, int]
	buckets   columnx.Column[model.Upstream, string]
	enabled   columnx.Column[model.Upstream, int]
	createdAt columnx.Column[model.Upstream, int64]
	updatedAt columnx.Column[model.Upstream, int64]
}

type metadataUpstreamsSchema struct {
	schemax.Schema[model.Upstream]
	ID        columnx.Column[model.Upstream, string] `dbx:"id,pk"`
	Name      columnx.Column[model.Upstream, string] `dbx:"name"`
	Endpoint  columnx.Column[model.Upstream, string] `dbx:"endpoint"`
	Region    columnx.Column[model.Upstream, string] `dbx:"region"`
	Weight    columnx.Column[model.Upstream, int]    `dbx:"weight"`
	Priority  columnx.Column[model.Upstream, int]    `dbx:"priority"`
	Buckets   columnx.Column[model.Upstream, string] `dbx:"buckets"`
	Enabled   columnx.Column[model.Upstream, int]    `dbx:"enabled"`
	CreatedAt columnx.Column[model.Upstream, int64]  `dbx:"created_at"`
	UpdatedAt columnx.Column[model.Upstream, int64]  `dbx:"updated_at"`
}

func newMetadataUpstreamsTable() metadataUpstreamsTable {
	schema := schemax.MustSchema("metadata_upstreams", metadataUpstreamsSchema{})
	return metadataUpstreamsTable{
		schema:    schema,
		id:        schema.ID,
		name:      schema.Name,
		endpoint:  schema.Endpoint,
		region:    schema.Region,
		weight:    schema.Weight,
		priority:  schema.Priority,
		buckets:   schema.Buckets,
		enabled:   schema.Enabled,
		createdAt: schema.CreatedAt,
		updatedAt: schema.UpdatedAt,
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

func (s *SQLMetadata) ListUpstreams(ctx context.Context) (*collectionlist.List[model.Upstream], error) {
	query := querydsl.SelectFrom(metadataUpstreams.schema, metadataUpstreams.selectItems()...).
		OrderBy(metadataUpstreams.priority.Asc(), metadataUpstreams.name.Asc(), metadataUpstreams.id.Asc())
	upstreams, err := s.repos.upstreams.List(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list upstreams: %w", err)
	}
	return upstreams, nil
}

func (s *SQLMetadata) GetUpstream(ctx context.Context, id string) (model.Upstream, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.Upstream{}, false, ErrBadRequest
	}

	query := querydsl.SelectFrom(metadataUpstreams.schema, metadataUpstreams.selectItems()...).
		Where(metadataUpstreams.id.Eq(id)).
		Limit(1)
	option, err := s.repos.upstreams.FirstOption(ctx, query)
	if err != nil {
		return model.Upstream{}, false, fmt.Errorf("query upstream: %w", err)
	}
	upstream, found := option.Get()
	return upstream, found, nil
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

	query := querydsl.InsertInto(metadataUpstreams.schema).
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

	query := querydsl.DeleteFrom(metadataUpstreams.schema).Where(metadataUpstreams.id.Eq(id))
	result, err := s.repos.upstreams.Delete(ctx, query)
	if err != nil {
		return false, fmt.Errorf("delete upstream: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete upstream rows: %w", err)
	}
	return affected > 0, nil
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
