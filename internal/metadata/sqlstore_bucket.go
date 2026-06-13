package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var metadataBuckets = newMetadataBucketsTable()

type metadataBucketsTable struct {
	table     querydsl.Table
	name      columnx.Column[struct{}, string]
	createdAt columnx.Column[struct{}, int64]
}

func newMetadataBucketsTable() metadataBucketsTable {
	table := querydsl.NewTable("metadata_buckets")
	return metadataBucketsTable{
		table:     table,
		name:      columnx.Named[string](table, "name"),
		createdAt: columnx.Named[int64](table, "created_at"),
	}
}

func (t metadataBucketsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{t.name, t.createdAt}
}

func (s *SQLMetadata) ListBuckets(ctx context.Context) ([]model.Bucket, error) {
	ctx = ensureContext(ctx)
	query := querydsl.SelectFrom(metadataBuckets.table, metadataBuckets.selectItems()...).
		OrderBy(metadataBuckets.name.Asc())
	rows, err := s.queryBuilderContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query buckets: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
			s.logger.Error("close sql metadata rows", "rows", "buckets", "error", closeErr)
		}
	}()

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

func (s *SQLMetadata) BucketExists(ctx context.Context, bucket string) (bool, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return false, ErrBadRequest
	}
	ctx = ensureContext(ctx)
	query := querydsl.SelectFrom(metadataBuckets.table, metadataBuckets.name).
		Where(metadataBuckets.name.Eq(bucket)).
		Limit(1)
	row, err := s.queryRowBuilderContext(ctx, query)
	if err != nil {
		return false, fmt.Errorf("query bucket exists: %w", err)
	}
	var found string
	err = row.Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("query bucket exists: %w", err)
	}
	return true, nil
}

func (s *SQLMetadata) CreateBucket(ctx context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}
	ctx = ensureContext(ctx)
	query := querydsl.InsertInto(metadataBuckets.table).
		Values(
			metadataBuckets.name.Set(bucket),
			metadataBuckets.createdAt.Set(time.Now().UTC().UnixNano()),
		)
	if _, err := s.execBuilderContext(ctx, query); err != nil {
		if isSQLConstraintError(err) {
			return ErrBucketExists
		}
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func (s *SQLMetadata) DeleteBucket(ctx context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}

	return s.withTx(ctx, "delete bucket", func(tx *sql.Tx) error {
		existsQuery := s.normalizeQuery(metadataBucketExistsQuery)
		deleteCommittedQuery := s.normalizeQuery("DELETE FROM metadata_objects WHERE bucket = ? AND state = ?")
		deletePendingQuery := s.normalizeQuery("DELETE FROM metadata_objects WHERE bucket = ? AND state = ?")
		deleteBucketQuery := s.normalizeQuery("DELETE FROM metadata_buckets WHERE name = ?")
		if err := ensureBucketInTx(ctx, tx, existsQuery, bucket); err != nil {
			return err
		}
		if err := s.decreaseBucketBlobRefsInTx(ctx, tx, bucket); err != nil {
			return err
		}
		if err := s.deleteBucketObjectsInTx(ctx, tx, deleteCommittedQuery, bucket, model.ObjectStateCommitted); err != nil {
			return err
		}
		if err := s.deleteBucketObjectsInTx(ctx, tx, deletePendingQuery, bucket, model.ObjectStatePending); err != nil {
			return err
		}
		return s.deleteBucketRowInTx(ctx, tx, deleteBucketQuery, bucket)
	})
}

func (s *SQLMetadata) ensureBucket(ctx context.Context, bucket string) error {
	exists, err := s.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("check bucket exists: %w", err)
	}
	if !exists {
		return ErrBucketNotFound
	}
	return nil
}

func (s *SQLMetadata) decreaseBucketBlobRefsInTx(ctx context.Context, tx *sql.Tx, bucket string) error {
	hashes, err := s.queryBucketCommittedHashesInTx(ctx, tx, bucket)
	if err != nil {
		return err
	}
	for _, hash := range hashes {
		if _, _, err := s.decreaseBlobRefInTx(ctx, tx, hash); err != nil && !errors.Is(err, ErrObjectNotFound) {
			return fmt.Errorf("decrease blob ref: %w", err)
		}
	}
	return nil
}

func (s *SQLMetadata) queryBucketCommittedHashesInTx(ctx context.Context, tx *sql.Tx, bucket string) ([]string, error) {
	rows, err := s.txQueryContext(
		ensureContext(ctx),
		tx,
		"SELECT hash FROM metadata_objects WHERE bucket = ? AND state = ?",
		bucket,
		model.ObjectStateCommitted,
	)
	if err != nil {
		return nil, fmt.Errorf("query bucket committed objects: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
			s.logger.Error("close sql metadata rows", "rows", "bucket object hashes", "error", closeErr)
		}
	}()

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

func ensureBucketInTx(ctx context.Context, tx *sql.Tx, query, bucket string) error {
	exists, err := bucketExistsInQuery(ctx, tx, query, bucket)
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
}, query, bucket string) (bool, error) {
	var exists int
	err := queryer.QueryRowContext(
		ensureContext(ctx),
		query,
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

func (s *SQLMetadata) deleteBucketObjectsInTx(ctx context.Context, tx *sql.Tx, query, bucket, state string) error {
	if err := s.txExecContext(
		ensureContext(ctx),
		tx,
		query,
		bucket,
		state,
	); err != nil {
		return fmt.Errorf("delete bucket objects: %w", err)
	}
	return nil
}

func (s *SQLMetadata) deleteBucketRowInTx(ctx context.Context, tx *sql.Tx, query, bucket string) error {
	if err := s.txExecContext(ensureContext(ctx), tx, query, bucket); err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	return nil
}

const metadataBucketExistsQuery = "SELECT 1 FROM metadata_buckets WHERE name = ? LIMIT 1"

func isSQLConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "duplicate key value violates unique constraint") ||
		strings.Contains(message, "duplicate entry") ||
		strings.Contains(message, "error 1062") ||
		strings.Contains(message, "constraint failed")
}
