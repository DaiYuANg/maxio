package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

func (s *SQLiteMetadata) ListBlobRefs(ctx context.Context) ([]BlobRef, error) {
	rows, err := s.db.QueryContext(
		ensureContext(ctx),
		`SELECT hash, path, size, ref_count, shard_placements, shard_checksums, shard_sizes
		   FROM metadata_blob_refs`,
	)
	if err != nil {
		return nil, fmt.Errorf("query blob refs: %w", err)
	}
	defer s.closeRows(rows, "blob refs")

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

func (s *SQLiteMetadata) GetBlobRef(ctx context.Context, hash string) (BlobRef, bool, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return BlobRef{}, false, ErrBadRequest
	}

	row := s.db.QueryRowContext(
		ensureContext(ctx),
		`SELECT hash, path, size, ref_count, shard_placements, shard_checksums, shard_sizes
		   FROM metadata_blob_refs
		  WHERE hash = ?`,
		hash,
	)
	ref, err := scanBlobRef(row)
	if errors.Is(err, sql.ErrNoRows) {
		return BlobRef{}, false, nil
	}
	if err != nil {
		return BlobRef{}, false, fmt.Errorf("get blob ref: %w", err)
	}
	return ref, true, nil
}

func (s *SQLiteMetadata) CreateBlobRef(
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

	if _, err := s.db.ExecContext(
		ensureContext(ctx),
		`INSERT INTO metadata_blob_refs (
			hash, path, size, ref_count, shard_placements, shard_checksums, shard_sizes
		) VALUES (?, ?, ?, 1, ?, ?, ?)
		ON CONFLICT(hash) DO NOTHING`,
		hash,
		path,
		size,
		marshalShardPlacements(placements),
		marshalStrings(checksums),
		marshalInt64sVariadic(shardSizes...),
	); err != nil {
		return fmt.Errorf("create blob ref: %w", err)
	}
	return nil
}

func (s *SQLiteMetadata) UpdateBlobRefPlacements(ctx context.Context, hash string, placements []model.ShardPlacement) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ErrBadRequest
	}
	result, err := s.db.ExecContext(
		ensureContext(ctx),
		"UPDATE metadata_blob_refs SET shard_placements = ? WHERE hash = ?",
		marshalShardPlacements(placements),
		hash,
	)
	if err != nil {
		return fmt.Errorf("update blob ref placements: %w", err)
	}
	return requireAffectedRow(result, ErrObjectNotFound, "update blob ref placements rows")
}

func (s *SQLiteMetadata) IncreaseBlobRef(ctx context.Context, hash string) error {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return ErrBadRequest
	}
	result, err := s.db.ExecContext(
		ensureContext(ctx),
		"UPDATE metadata_blob_refs SET ref_count = ref_count + 1 WHERE hash = ?",
		hash,
	)
	if err != nil {
		return fmt.Errorf("increase blob ref: %w", err)
	}
	return requireAffectedRow(result, ErrObjectNotFound, "increase blob ref rows")
}

func (s *SQLiteMetadata) DecreaseBlobRef(ctx context.Context, hash string) (string, bool, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return "", false, ErrBadRequest
	}

	var path string
	var removed bool
	err := s.withTx(ctx, "decrease blob ref", func(tx *sql.Tx) error {
		var err error
		path, removed, err = decreaseBlobRefInTx(ctx, tx, hash)
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

func decreaseBlobRefInTx(ctx context.Context, tx *sql.Tx, hash string) (string, bool, error) {
	var path string
	var refCount int
	err := tx.QueryRowContext(
		ensureContext(ctx),
		"SELECT path, ref_count FROM metadata_blob_refs WHERE hash = ?",
		hash,
	).Scan(&path, &refCount)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrObjectNotFound
	}
	if err != nil {
		return "", false, fmt.Errorf("query blob ref: %w", err)
	}
	if refCount <= 1 {
		return path, true, deleteBlobRefInTx(ctx, tx, hash)
	}
	return path, false, updateBlobRefCountInTx(ctx, tx, hash)
}

func deleteBlobRefInTx(ctx context.Context, tx *sql.Tx, hash string) error {
	if _, err := tx.ExecContext(ensureContext(ctx), "DELETE FROM metadata_blob_refs WHERE hash = ?", hash); err != nil {
		return fmt.Errorf("delete blob ref: %w", err)
	}
	return nil
}

func updateBlobRefCountInTx(ctx context.Context, tx *sql.Tx, hash string) error {
	if _, err := tx.ExecContext(
		ensureContext(ctx),
		"UPDATE metadata_blob_refs SET ref_count = ref_count - 1 WHERE hash = ?",
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
