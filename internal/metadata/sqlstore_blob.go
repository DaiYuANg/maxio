package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/querydsl"
)

func (s *SQLMetadata) ListBlobRefs(ctx context.Context) (*collectionlist.List[BlobRef], error) {
	query := querydsl.SelectFrom(metadataBlobRefs.schema, metadataBlobRefs.selectItems()...)
	refs, err := s.repos.blobRefs.List(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list blob refs: %w", err)
	}
	return refs, nil
}

func (s *SQLMetadata) GetBlobRef(ctx context.Context, hash string) (BlobRef, bool, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return BlobRef{}, false, ErrBadRequest
	}

	query := querydsl.SelectFrom(metadataBlobRefs.schema, metadataBlobRefs.selectItems()...).
		Where(metadataBlobRefs.hash.Eq(hash)).
		Limit(1)
	option, err := s.repos.blobRefs.FirstOption(ctx, query)
	if err != nil {
		return BlobRef{}, false, fmt.Errorf("query blob ref: %w", err)
	}
	ref, found := option.Get()
	return ref, found, nil
}

func (s *SQLMetadata) CreateBlobRef(
	ctx context.Context,
	hash string,
	path string,
	size int64,
) error {
	hash = strings.TrimSpace(hash)
	path = strings.TrimSpace(path)
	if hash == "" || path == "" || size < 0 {
		return ErrBadRequest
	}

	ref := BlobRef{Hash: hash, Path: path, Size: size, RefCount: 1}
	assignments, err := s.repos.blobRefs.Mapper().InsertAssignmentsWithID(ctx, metadataBlobRefs.schema, &ref, nil)
	if err != nil {
		return fmt.Errorf("map blob ref insert assignments: %w", err)
	}
	query := querydsl.InsertInto(metadataBlobRefs.schema).
		ValuesList(assignments).
		OnConflict(metadataBlobRefs.hash).
		DoNothing()
	if _, err := dbx.Exec(ensureContext(ctx), s.dbxDB, query); err != nil {
		return fmt.Errorf("create blob ref: %w", err)
	}
	return nil
}

func (s *SQLMetadata) IncreaseBlobRef(ctx context.Context, hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ErrBadRequest
	}
	result, err := s.execSQLTemplateContext(ctx, metadataSQLBlobIncreaseRefCount, metadataBlobRefHashParams{Hash: hash})
	if err != nil {
		return fmt.Errorf("increase blob ref: %w", err)
	}
	return requireAffectedRow(result, ErrObjectNotFound, "increase blob ref rows")
}

func (s *SQLMetadata) DecreaseBlobRef(ctx context.Context, hash string) (string, bool, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", false, ErrBadRequest
	}

	var path string
	var removed bool
	err := s.withTx(ctx, "decrease blob ref", func(tx *dbx.Tx) error {
		var err error
		path, removed, err = s.decreaseBlobRefInTx(ctx, tx, hash)
		return err
	})
	return path, removed, err
}

func (s *SQLMetadata) decreaseBlobRefInTx(ctx context.Context, tx *dbx.Tx, hash string) (string, bool, error) {
	query := querydsl.SelectFrom(metadataBlobRefs.schema, metadataBlobRefs.path, metadataBlobRefs.refCount).
		Where(metadataBlobRefs.hash.Eq(hash)).
		Limit(1)
	option, err := dbx.QueryOption(ensureContext(ctx), tx, query, metadataBlobRefCounterMapper)
	if err != nil {
		return "", false, fmt.Errorf("query blob ref: %w", err)
	}
	ref, found := option.Get()
	if !found {
		return "", false, ErrObjectNotFound
	}
	if ref.RefCount <= 1 {
		return ref.Path, true, s.deleteBlobRefInTx(ctx, tx, hash)
	}
	return ref.Path, false, s.updateBlobRefCountInTx(ctx, tx, hash)
}

func (s *SQLMetadata) deleteBlobRefInTx(ctx context.Context, tx *dbx.Tx, hash string) error {
	query := querydsl.DeleteFrom(metadataBlobRefs.schema).
		Where(metadataBlobRefs.hash.Eq(hash))
	if _, err := dbx.Exec(ensureContext(ctx), tx, query); err != nil {
		return fmt.Errorf("delete blob ref: %w", err)
	}
	return nil
}

func (s *SQLMetadata) updateBlobRefCountInTx(ctx context.Context, tx *dbx.Tx, hash string) error {
	if err := s.txExecSQLTemplateContext(ctx, tx, metadataSQLBlobDecreaseRefCount, metadataBlobRefHashParams{Hash: hash}); err != nil {
		return fmt.Errorf("decrease blob ref: %w", err)
	}
	return nil
}

func requireAffectedRow(result sql.Result, missing error, op string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if affected == 0 {
		return missing
	}
	return nil
}
