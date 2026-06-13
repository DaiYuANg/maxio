package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
)

var metadataDigestRefs = newMetadataDigestRefsTable()

type metadataDigestRefsTable struct {
	table          querydsl.Table
	digest         columnx.Column[struct{}, string]
	size           columnx.Column[struct{}, int64]
	refCount       columnx.Column[struct{}, int]
	upstreamID     columnx.Column[struct{}, string]
	upstreamBucket columnx.Column[struct{}, string]
	upstreamKey    columnx.Column[struct{}, string]
	createdAt      columnx.Column[struct{}, int64]
	updatedAt      columnx.Column[struct{}, int64]
}

func newMetadataDigestRefsTable() metadataDigestRefsTable {
	table := querydsl.NewTable("metadata_digest_refs")
	return metadataDigestRefsTable{
		table:          table,
		digest:         columnx.Named[string](table, "digest"),
		size:           columnx.Named[int64](table, "size"),
		refCount:       columnx.Named[int](table, "ref_count"),
		upstreamID:     columnx.Named[string](table, "upstream_id"),
		upstreamBucket: columnx.Named[string](table, "upstream_bucket"),
		upstreamKey:    columnx.Named[string](table, "upstream_key"),
		createdAt:      columnx.Named[int64](table, "created_at"),
		updatedAt:      columnx.Named[int64](table, "updated_at"),
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
