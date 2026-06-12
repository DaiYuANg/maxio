package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/model"
)

func (s *SQLiteMetadata) ListBuckets(ctx context.Context) ([]model.Bucket, error) {
	ctx = ensureContext(ctx)
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT name, created_at
		   FROM metadata_buckets
		  ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query buckets: %w", err)
	}
	defer s.closeRows(rows, "buckets")

	buckets := make([]model.Bucket, 0)
	for rows.Next() {
		bucket, err := scanBucket(rows)
		if err != nil {
			return nil, err
		}
		buckets = append(buckets, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate buckets: %w", err)
	}
	return buckets, nil
}

func (s *SQLiteMetadata) BucketExists(ctx context.Context, bucket string) (bool, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return false, ErrBadRequest
	}
	ctx = ensureContext(ctx)
	return bucketExistsInQuery(ctx, s.db, bucket)
}

func (s *SQLiteMetadata) CreateBucket(ctx context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}
	ctx = ensureContext(ctx)
	if _, err := s.db.ExecContext(
		ctx,
		"INSERT INTO metadata_buckets (name, created_at) VALUES (?, ?)",
		bucket,
		time.Now().UTC().UnixNano(),
	); err != nil {
		if isSQLiteConstraintError(err) {
			return ErrBucketExists
		}
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func (s *SQLiteMetadata) DeleteBucket(ctx context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}

	return s.withTx(ctx, "delete bucket", func(tx *sql.Tx) error {
		if err := ensureBucketInTx(ctx, tx, bucket); err != nil {
			return err
		}
		if err := s.decreaseBucketBlobRefsInTx(ctx, tx, bucket); err != nil {
			return err
		}
		if err := deleteBucketObjectsInTx(ctx, tx, bucket, model.ObjectStateCommitted); err != nil {
			return err
		}
		if err := deleteBucketObjectsInTx(ctx, tx, bucket, model.ObjectStatePending); err != nil {
			return err
		}
		return deleteBucketRowInTx(ctx, tx, bucket)
	})
}

func (s *SQLiteMetadata) ensureBucket(ctx context.Context, bucket string) error {
	exists, err := s.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check bucket exists: %w", err)
	}
	if !exists {
		return ErrBucketNotFound
	}
	return nil
}

func (s *SQLiteMetadata) decreaseBucketBlobRefsInTx(ctx context.Context, tx *sql.Tx, bucket string) error {
	hashes, err := s.queryBucketCommittedHashesInTx(ctx, tx, bucket)
	if err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, _, err := decreaseBlobRefInTx(ctx, tx, hash); err != nil && !errors.Is(err, ErrObjectNotFound) {
			return fmt.Errorf("decrease blob ref: %w", err)
		}
	}
	return nil
}

func (s *SQLiteMetadata) queryBucketCommittedHashesInTx(ctx context.Context, tx *sql.Tx, bucket string) ([]string, error) {
	rows, err := tx.QueryContext(
		ensureContext(ctx),
		"SELECT hash FROM metadata_objects WHERE bucket = ? AND state = ?",
		bucket,
		model.ObjectStateCommitted,
	)
	if err != nil {
		return nil, fmt.Errorf("query bucket committed objects: %w", err)
	}
	defer s.closeRows(rows, "bucket object hashes")

	hashes := make([]string, 0)
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("scan object hash: %w", err)
		}
		hashes = append(hashes, hash)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bucket object hashes: %w", err)
	}
	return hashes, nil
}

func scanBucket(scanner interface{ Scan(dest ...any) error }) (model.Bucket, error) {
	var name string
	var createdAt int64
	if err := scanner.Scan(&name, &createdAt); err != nil {
		return model.Bucket{}, fmt.Errorf("scan bucket: %w", err)
	}
	return model.Bucket{
		Name:      name,
		CreatedAt: unixNanoToTime(createdAt),
	}, nil
}

func ensureBucketInTx(ctx context.Context, tx *sql.Tx, bucket string) error {
	exists, err := bucketExistsInQuery(ctx, tx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		return ErrBucketNotFound
	}
	return nil
}

func bucketExistsInQuery(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, bucket string) (bool, error) {
	var exists int
	err := queryer.QueryRowContext(
		ensureContext(ctx),
		"SELECT 1 FROM metadata_buckets WHERE name = ? LIMIT 1",
		bucket,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query bucket exists: %w", err)
	}
	return true, nil
}

func deleteBucketObjectsInTx(ctx context.Context, tx *sql.Tx, bucket, state string) error {
	if _, err := tx.ExecContext(
		ensureContext(ctx),
		"DELETE FROM metadata_objects WHERE bucket = ? AND state = ?",
		bucket,
		state,
	); err != nil {
		return fmt.Errorf("delete bucket objects: %w", err)
	}
	return nil
}

func deleteBucketRowInTx(ctx context.Context, tx *sql.Tx, bucket string) error {
	if _, err := tx.ExecContext(ensureContext(ctx), "DELETE FROM metadata_buckets WHERE name = ?", bucket); err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	return nil
}

func isSQLiteConstraintError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "UNIQUE constraint failed") || strings.Contains(message, "constraint failed")
}
