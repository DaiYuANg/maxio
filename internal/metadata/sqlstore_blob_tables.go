package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	schemax "github.com/arcgolabs/dbx/schema"
)

var (
	metadataBlobRefs = newMetadataBlobRefsTable()
)

type metadataBlobRefsTable struct {
	schema   metadataBlobRefsSchema
	hash     columnx.Column[BlobRef, string]
	path     columnx.Column[BlobRef, string]
	size     columnx.Column[BlobRef, int64]
	refCount columnx.Column[BlobRef, int]
}

type metadataBlobRefsSchema struct {
	schemax.Schema[BlobRef]
	Hash     columnx.Column[BlobRef, string] `dbx:"hash,pk"`
	Path     columnx.Column[BlobRef, string] `dbx:"path"`
	Size     columnx.Column[BlobRef, int64]  `dbx:"size"`
	RefCount columnx.Column[BlobRef, int]    `dbx:"ref_count"`
}

func newMetadataBlobRefsTable() metadataBlobRefsTable {
	schema := schemax.MustSchema("metadata_blob_refs", metadataBlobRefsSchema{})
	return metadataBlobRefsTable{
		schema:   schema,
		hash:     schema.Hash,
		path:     schema.Path,
		size:     schema.Size,
		refCount: schema.RefCount,
	}
}

func (t metadataBlobRefsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{
		t.hash,
		t.path,
		t.size,
		t.refCount,
	}
}
