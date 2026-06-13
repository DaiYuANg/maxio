package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/arcgolabs/dbx/querydsl"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) ListBlobRefs(ctx context.Context) ([]BlobRef, error) {
	query := querydsl.SelectFrom(metadataBlobRefs.table, metadataBlobRefs.selectItems()...)
	rows, err := s.queryBuilderContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query blob refs: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
			s.logger.Error("close sql metadata rows", "rows", "blob refs", "error", closeErr)
		}
	}()

	refs := make([]BlobRef, 0)
	for rows.Next() {
		ref, err := scanBlobRef(rows)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate blob refs: %w", err)
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
	placements []model.ShardPlacement,
	checksums []string,
	shardSizes ...[]int64,
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
			metadataBlobRefs.shardPlacements.Set(marshalShardPlacements(placements)),
			metadataBlobRefs.shardChecksums.Set(marshalStrings(checksums)),
			metadataBlobRefs.shardSizes.Set(marshalInt64sVariadic(shardSizes...)),
		).
		OnConflict(metadataBlobRefs.hash).
		DoNothing()
	if _, err := s.execBuilderContext(ctx, query); err != nil {
		return fmt.Errorf("create blob ref: %w", err)
	}
	return nil
}

func (s *SQLMetadata) UpdateBlobRefPlacements(ctx context.Context, hash string, placements []model.ShardPlacement) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ErrBadRequest
	}
	query := querydsl.Update(metadataBlobRefs.table).
		Set(metadataBlobRefs.shardPlacements.Set(marshalShardPlacements(placements))).
		Where(metadataBlobRefs.hash.Eq(hash))
	result, err := s.execBuilderContext(ctx, query)
	if err != nil {
		return fmt.Errorf("update blob ref placements: %w", err)
	}
	return requireAffectedRow(result, ErrObjectNotFound, "update blob ref placements rows")
}

func (s *SQLMetadata) IncreaseBlobRef(ctx context.Context, hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ErrBadRequest
	}
	result, err := s.execContext(
		ctx,
		"UPDATE metadata_blob_refs SET ref_count = ref_count + 1 WHERE hash = ?",
		hash,
	)
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
		ref        BlobRef
		placements sql.NullString
		checksums  sql.NullString
		shardSizes sql.NullString
	)
	if err := scanner.Scan(
		&ref.Hash,
		&ref.Path,
		&ref.Size,
		&ref.RefCount,
		&placements,
		&checksums,
		&shardSizes,
	); err != nil {
		return BlobRef{}, fmt.Errorf("scan blob ref: %w", err)
	}
	if err := decodeJSON(placements, &ref.ShardPlacements); err != nil {
		return BlobRef{}, fmt.Errorf("decode placements: %w", err)
	}
	if err := decodeJSON(checksums, &ref.ShardChecksums); err != nil {
		return BlobRef{}, fmt.Errorf("decode checksums: %w", err)
	}
	if err := decodeJSON(shardSizes, &ref.ShardSizes); err != nil {
		return BlobRef{}, fmt.Errorf("decode shard sizes: %w", err)
	}
	return ref, nil
}

func (s *SQLMetadata) decreaseBlobRefInTx(ctx context.Context, tx *sql.Tx, hash string) (string, bool, error) {
	var path string
	var refCount int
	err := s.txQueryRowContext(
		ctx,
		tx,
		s.normalizeQuery("SELECT path, ref_count FROM metadata_blob_refs WHERE hash = ?"),
		hash,
	).Scan(&path, &refCount)
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
	if err := s.txExecContext(
		ensureContext(ctx),
		tx,
		s.normalizeQuery("DELETE FROM metadata_blob_refs WHERE hash = ?"),
		hash,
	); err != nil {
		return fmt.Errorf("delete blob ref: %w", err)
	}
	return nil
}

func (s *SQLMetadata) updateBlobRefCountInTx(ctx context.Context, tx *sql.Tx, hash string) error {
	if err := s.txExecContext(
		ensureContext(ctx),
		tx,
		s.normalizeQuery("UPDATE metadata_blob_refs SET ref_count = ref_count - 1 WHERE hash = ?"),
		hash,
	); err != nil {
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
