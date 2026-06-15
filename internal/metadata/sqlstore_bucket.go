package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
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

func (s *SQLMetadata) ListBuckets(ctx context.Context) (*collectionlist.List[model.Bucket], error) {
	ctx = ensureContext(ctx)
	query := querydsl.SelectFrom(metadataBuckets.table, metadataBuckets.selectItems()...).
		OrderBy(metadataBuckets.name.Asc())
	buckets, err := querySQLRows(
		ctx,
		s,
		query,
		"buckets",
		metadataBucketMapper,
	)
	if err != nil {
		return nil, err
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
	_, found, err := querySQLOne(ctx, s, query, "bucket exists", metadataNameMapper)
	if err != nil {
		return false, err
	}
	return found, nil
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
		if err := s.ensureBucketInTx(ctx, tx, bucket); err != nil {
			return err
		}
		if err := s.decreaseBucketBlobRefsInTx(ctx, tx, bucket); err != nil {
			return err
		}
		if err := s.deleteBucketObjectsInTx(ctx, tx, bucket, model.ObjectStateCommitted); err != nil {
			return err
		}
		if err := s.deleteBucketObjectsInTx(ctx, tx, bucket, model.ObjectStatePending); err != nil {
			return err
		}
		return s.deleteBucketRowInTx(ctx, tx, bucket)
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
	var queryErr error
	hashes.Range(func(_ int, hash string) bool {
		if _, _, err := s.decreaseBlobRefInTx(ctx, tx, hash); err != nil && !errors.Is(err, ErrObjectNotFound) {
			queryErr = fmt.Errorf("decrease blob ref: %w", err)
			return false
		}
		return true
	})
	if queryErr != nil {
		return queryErr
	}
	return nil
}

func (s *SQLMetadata) queryBucketCommittedHashesInTx(ctx context.Context, tx *sql.Tx, bucket string) (*collectionlist.List[string], error) {
	query := querydsl.SelectFrom(metadataObjects.table, metadataObjects.hash).
		Where(querydsl.And(metadataObjects.bucket.Eq(bucket), metadataObjects.state.Eq(model.ObjectStateCommitted)))
	hashes, err := querySQLRowsInTx(ctx, s, tx, query, "bucket object hashes", metadataHashMapper)
	if err != nil {
		return nil, err
	}
	return metadataHashRowsToList(hashes), nil
}

func (s *SQLMetadata) ensureBucketInTx(ctx context.Context, tx *sql.Tx, bucket string) error {
	exists, err := s.bucketExistsInTx(ctx, tx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		return ErrBucketNotFound
	}
	return nil
}

func (s *SQLMetadata) bucketExistsInTx(ctx context.Context, tx *sql.Tx, bucket string) (bool, error) {
	query := querydsl.SelectFrom(metadataBuckets.table, metadataBuckets.name).
		Where(metadataBuckets.name.Eq(bucket)).
		Limit(1)
	_, found, err := querySQLOneInTx(ctx, s, tx, query, "bucket exists", metadataNameMapper)
	if err != nil {
		return false, err
	}
	return found, nil
}

func (s *SQLMetadata) deleteBucketObjectsInTx(ctx context.Context, tx *sql.Tx, bucket, state string) error {
	query := querydsl.DeleteFrom(metadataObjects.table).
		Where(querydsl.And(metadataObjects.bucket.Eq(bucket), metadataObjects.state.Eq(state)))
	if err := s.txExecBuilderContext(ctx, tx, query); err != nil {
		return fmt.Errorf("delete bucket objects: %w", err)
	}
	return nil
}

func (s *SQLMetadata) deleteBucketRowInTx(ctx context.Context, tx *sql.Tx, bucket string) error {
	query := querydsl.DeleteFrom(metadataBuckets.table).
		Where(metadataBuckets.name.Eq(bucket))
	if err := s.txExecBuilderContext(ctx, tx, query); err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	return nil
}

func isSQLConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "duplicate key value violates unique constraint") ||
		strings.Contains(message, "duplicate entry") ||
		strings.Contains(message, "error 1062") ||
		strings.Contains(message, "constraint failed")
}
