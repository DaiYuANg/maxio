package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx/querydsl"
)

func (s *SQLMetadata) ListBlobRefs(ctx context.Context) (*collectionlist.List[BlobRef], error) {
	query := querydsl.SelectFrom(metadataBlobRefs.table, metadataBlobRefs.selectItems()...)
	refs, err := listSQLRows(
		ctx,
		s,
		query,
		"blob refs",
		scanBlobRef,
	)
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func (s *SQLMetadata) GetBlobRef(ctx context.Context, hash string) (BlobRef, bool, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return BlobRef{}, false, ErrBadRequest
	}

	query := querydsl.SelectFrom(metadataBlobRefs.table, metadataBlobRefs.selectItems()...).
		Where(metadataBlobRefs.hash.Eq(hash)).
		Limit(1)
	row, err := s.queryRowBuilderContext(ctx, query)
	if err != nil {
		return BlobRef{}, false, fmt.Errorf("get blob ref: %w", err)
	}
	ref, err := scanBlobRef(row)
	if errors.Is(err, sql.ErrNoRows) {
		return BlobRef{}, false, nil
	}
	if err != nil {
		return BlobRef{}, false, fmt.Errorf("get blob ref: %w", err)
	}
	return ref, true, nil
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

	query := querydsl.InsertInto(metadataBlobRefs.table).
		Values(
			metadataBlobRefs.hash.Set(hash),
			metadataBlobRefs.path.Set(path),
			metadataBlobRefs.size.Set(size),
			metadataBlobRefs.refCount.Set(1),
		).
		OnConflict(metadataBlobRefs.hash).
		DoNothing()
	if _, err := s.execBuilderContext(ctx, query); err != nil {
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
	err := s.withTx(ctx, "decrease blob ref", func(tx *sql.Tx) error {
		var err error
		path, removed, err = s.decreaseBlobRefInTx(ctx, tx, hash)
		return err
	})
	return path, removed, err
}

func scanBlobRef(scanner interface{ Scan(dest ...any) error }) (BlobRef, error) {
	var (
		ref BlobRef
	)
	if err := scanner.Scan(
		&ref.Hash,
		&ref.Path,
		&ref.Size,
		&ref.RefCount,
	); err != nil {
		return BlobRef{}, fmt.Errorf("scan blob ref: %w", err)
	}
	return ref, nil
}

func (s *SQLMetadata) decreaseBlobRefInTx(ctx context.Context, tx *sql.Tx, hash string) (string, bool, error) {
	var path string
	var refCount int
	query := querydsl.SelectFrom(metadataBlobRefs.table, metadataBlobRefs.path, metadataBlobRefs.refCount).
		Where(metadataBlobRefs.hash.Eq(hash)).
		Limit(1)
	row, queryErr := s.txQueryRowBuilderContext(ctx, tx, query)
	if queryErr != nil {
		return "", false, fmt.Errorf("query blob ref: %w", queryErr)
	}
	err := row.Scan(&path, &refCount)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrObjectNotFound
	}
	if err != nil {
		return "", false, fmt.Errorf("query blob ref: %w", err)
	}
	if refCount <= 1 {
		return path, true, s.deleteBlobRefInTx(ctx, tx, hash)
	}
	return path, false, s.updateBlobRefCountInTx(ctx, tx, hash)
}

func (s *SQLMetadata) deleteBlobRefInTx(ctx context.Context, tx *sql.Tx, hash string) error {
	query := querydsl.DeleteFrom(metadataBlobRefs.table).
		Where(metadataBlobRefs.hash.Eq(hash))
	if err := s.txExecBuilderContext(ctx, tx, query); err != nil {
		return fmt.Errorf("delete blob ref: %w", err)
	}
	return nil
}

func (s *SQLMetadata) updateBlobRefCountInTx(ctx context.Context, tx *sql.Tx, hash string) error {
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
