package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
)

var metadataBlobRefs = newMetadataBlobRefsTable()

type metadataBlobRefsTable struct {
	table           querydsl.Table
	hash            columnx.Column[struct{}, string]
	path            columnx.Column[struct{}, string]
	size            columnx.Column[struct{}, int64]
	refCount        columnx.Column[struct{}, int]
	shardPlacements columnx.Column[struct{}, string]
	shardChecksums  columnx.Column[struct{}, string]
	shardSizes      columnx.Column[struct{}, string]
}

func newMetadataBlobRefsTable() metadataBlobRefsTable {
	table := querydsl.NewTable("metadata_blob_refs")
	return metadataBlobRefsTable{
		table:           table,
		hash:            columnx.Named[string](table, "hash"),
		path:            columnx.Named[string](table, "path"),
		size:            columnx.Named[int64](table, "size"),
		refCount:        columnx.Named[int](table, "ref_count"),
		shardPlacements: columnx.Named[string](table, "shard_placements"),
		shardChecksums:  columnx.Named[string](table, "shard_checksums"),
		shardSizes:      columnx.Named[string](table, "shard_sizes"),
	}
}

func (t metadataBlobRefsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.hash,
		t.path,
		t.size,
		t.refCount,
		t.shardPlacements,
		t.shardChecksums,
		t.shardSizes,
	}
}
