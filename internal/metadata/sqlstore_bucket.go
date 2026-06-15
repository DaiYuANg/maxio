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
	schemax "github.com/arcgolabs/dbx/schema"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var (
	metadataBuckets = newMetadataBucketsTable()
)

type metadataBucketsTable struct {
	schema    metadataBucketsSchema
	name      columnx.Column[model.Bucket, string]
	createdAt columnx.Column[model.Bucket, int64]
}

type metadataBucketsSchema struct {
	schemax.Schema[model.Bucket]
	Name      columnx.Column[model.Bucket, string] `dbx:"name,pk"`
	CreatedAt columnx.Column[model.Bucket, int64]  `dbx:"created_at"`
}

func newMetadataBucketsTable() metadataBucketsTable {
	schema := schemax.MustSchema("metadata_buckets", metadataBucketsSchema{})
	return metadataBucketsTable{
		schema:    schema,
		name:      schema.Name,
		createdAt: schema.CreatedAt,
	}
}

func (t metadataBucketsTable) selectItems() []querydsl.SelectItem {
	return []querydsl.SelectItem{t.name, t.createdAt}
}

func (s *SQLMetadata) ListBuckets(ctx context.Context) (*collectionlist.List[model.Bucket], error) {
	ctx = ensureContext(ctx)
	query := querydsl.SelectFrom(metadataBuckets.schema, metadataBuckets.selectItems()...).
		OrderBy(metadataBuckets.name.Asc())
	buckets, err := s.repos.buckets.List(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	return buckets, nil
}

func (s *SQLMetadata) BucketExists(ctx context.Context, bucket string) (bool, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return false, ErrBadRequest
	}
	ctx = ensureContext(ctx)
	query := querydsl.SelectFrom(metadataBuckets.schema).
		Where(metadataBuckets.name.Eq(bucket))
	found, err := s.repos.buckets.Exists(ctx, query)
	if err != nil {
		return false, fmt.Errorf("check bucket exists: %w", err)
	}
	return found, nil
}

func (s *SQLMetadata) CreateBucket(ctx context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}
	ctx = ensureContext(ctx)
	err := s.repos.buckets.Create(ctx, &model.Bucket{Name: bucket, CreatedAt: time.Now().UTC()})
	if err != nil {
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
	query := querydsl.SelectValue(metadataObjects.hash).
		From(metadataObjects.schema).
		Where(querydsl.And(metadataObjects.bucket.Eq(bucket), metadataObjects.state.Eq(model.ObjectStateCommitted)))
	hashes, err := querySQLScalarsInTx(ctx, s, tx, query, "bucket object hashes")
	if err != nil {
		return nil, err
	}
	return hashes, nil
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
	query := querydsl.SelectValue(metadataBuckets.name).
		From(metadataBuckets.schema).
		Where(metadataBuckets.name.Eq(bucket)).
		Limit(1)
	_, found, err := querySQLScalarOptionInTx(ctx, s, tx, query, "bucket exists")
	if err != nil {
		return false, err
	}
	return found, nil
}

func (s *SQLMetadata) deleteBucketObjectsInTx(ctx context.Context, tx *sql.Tx, bucket, state string) error {
	query := querydsl.DeleteFrom(metadataObjects.schema).
		Where(querydsl.And(metadataObjects.bucket.Eq(bucket), metadataObjects.state.Eq(state)))
	if err := s.txExecBuilderContext(ctx, tx, query); err != nil {
		return fmt.Errorf("delete bucket objects: %w", err)
	}
	return nil
}

func (s *SQLMetadata) deleteBucketRowInTx(ctx context.Context, tx *sql.Tx, bucket string) error {
	query := querydsl.DeleteFrom(metadataBuckets.schema).
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
