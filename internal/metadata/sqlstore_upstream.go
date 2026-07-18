package metadata

import (
	"context"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/arcgolabs/dbx"
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
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

func (s *SQLMetadata) ListUpstreams(ctx context.Context) (*collectionlist.List[model.Upstream], error) {
	upstreams, err := s.repos.upstreams.ListSpec(
		ctx,
		repositoryx.OrderBy(metadataUpstreams.priority.Asc(), metadataUpstreams.name.Asc(), metadataUpstreams.id.Asc()),
	)
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

	return getRepositoryByKey[model.Upstream](
		ctx,
		s.repos.upstreams,
		repositoryx.KeySet(repositoryx.Part(metadataUpstreams.id, id)),
		"query upstream",
	)
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

	assignments, err := s.repos.upstreams.Mapper().InsertAssignmentsWithID(ctx, metadataUpstreams.schema, &upstream, nil)
	if err != nil {
		return model.Upstream{}, fmt.Errorf("map upstream insert assignments: %w", err)
	}
	query := querydsl.InsertInto(metadataUpstreams.schema).
		ValuesList(assignments).
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

	_, execErr := dbx.Exec(ensureContext(ctx), s.dbxDB, query)
	if execErr != nil {
		return model.Upstream{}, fmt.Errorf("upsert upstream: %w", execErr)
	}
	return requireStoredEntity(s.GetUpstream(ctx, upstream.ID))
}

func (s *SQLMetadata) DeleteUpstream(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrBadRequest
	}

	return deleteRepositoryByKey[model.Upstream](
		ctx,
		s.repos.upstreams,
		repositoryx.KeySet(repositoryx.Part(metadataUpstreams.id, id)),
		"delete upstream",
	)
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
	normalized := collectionlist.FilterMapList(
		collectionlist.NewList(values...),
		func(_ int, value string) (string, bool) {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				return "", false
			}
			return trimmed, true
		},
	)
	unique := collectionset.NewOrderedSetWithCapacity[string](normalized.Len(), normalized.Values()...)
	return unique.Values()
}
