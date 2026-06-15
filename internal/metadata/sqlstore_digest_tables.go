package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	schemax "github.com/arcgolabs/dbx/schema"

	"github.com/lyonbrown4d/maxio/internal/model"
)

const metadataDigestRefsTableName = "metadata_digest_refs"

var (
	metadataDigestRefs = newMetadataDigestRefsTable()
)

type metadataDigestRefsTable struct {
	schema         metadataDigestRefsSchema
	digest         columnx.Column[model.DigestRef, string]
	size           columnx.Column[model.DigestRef, int64]
	refCount       columnx.Column[model.DigestRef, int]
	upstreamID     columnx.Column[model.DigestRef, string]
	upstreamBucket columnx.Column[model.DigestRef, string]
	upstreamKey    columnx.Column[model.DigestRef, string]
	createdAt      columnx.Column[model.DigestRef, int64]
	updatedAt      columnx.Column[model.DigestRef, int64]
}

type metadataDigestRefsSchema struct {
	schemax.Schema[model.DigestRef]
	Digest         columnx.Column[model.DigestRef, string] `dbx:"digest,pk"`
	Size           columnx.Column[model.DigestRef, int64]  `dbx:"size"`
	RefCount       columnx.Column[model.DigestRef, int]    `dbx:"ref_count"`
	UpstreamID     columnx.Column[model.DigestRef, string] `dbx:"upstream_id"`
	UpstreamBucket columnx.Column[model.DigestRef, string] `dbx:"upstream_bucket"`
	UpstreamKey    columnx.Column[model.DigestRef, string] `dbx:"upstream_key"`
	CreatedAt      columnx.Column[model.DigestRef, int64]  `dbx:"created_at"`
	UpdatedAt      columnx.Column[model.DigestRef, int64]  `dbx:"updated_at"`
}

func newMetadataDigestRefsTable() metadataDigestRefsTable {
	schema := schemax.MustSchema(metadataDigestRefsTableName, metadataDigestRefsSchema{})
	return metadataDigestRefsTable{
		schema:         schema,
		digest:         schema.Digest,
		size:           schema.Size,
		refCount:       schema.RefCount,
		upstreamID:     schema.UpstreamID,
		upstreamBucket: schema.UpstreamBucket,
		upstreamKey:    schema.UpstreamKey,
		createdAt:      schema.CreatedAt,
		updatedAt:      schema.UpdatedAt,
	}
}

func (t metadataDigestRefsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.digest,
		t.size,
		t.refCount,
		t.upstreamID,
		t.upstreamBucket,
		t.upstreamKey,
		t.createdAt,
		t.updatedAt,
	}
}
