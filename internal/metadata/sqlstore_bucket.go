package metadata

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx"
	columnx "github.com/arcgolabs/dbx/column"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
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

func (s *SQLMetadata) ListBuckets(ctx context.Context) (*collectionlist.List[model.Bucket], error) {
	ctx = ensureContext(ctx)
	buckets, err := s.repos.buckets.ListSpec(ctx, repositoryx.OrderBy(metadataBuckets.name.Asc()))
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
	found, err := s.repos.buckets.ExistsSpec(ctx, repositoryx.Where(metadataBuckets.name.Eq(bucket)))
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

	return s.withTx(ctx, "delete bucket", func(tx *dbx.Tx) error {
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

func (s *SQLMetadata) decreaseBucketBlobRefsInTx(ctx context.Context, tx *dbx.Tx, bucket string) error {
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

func (s *SQLMetadata) queryBucketCommittedHashesInTx(ctx context.Context, tx *dbx.Tx, bucket string) (*collectionlist.List[string], error) {
	query := querydsl.SelectValue(metadataObjects.hash).
		From(metadataObjects.schema).
		Where(querydsl.And(metadataObjects.bucket.Eq(bucket), metadataObjects.state.Eq(model.ObjectStateCommitted)))
	hashes, err := dbx.QueryTyped[string](ensureContext(ctx), tx, query)
	if err != nil {
		return nil, fmt.Errorf("query bucket object hashes: %w", err)
	}
	return hashes, nil
}

func (s *SQLMetadata) ensureBucketInTx(ctx context.Context, tx *dbx.Tx, bucket string) error {
	exists, err := s.bucketExistsInTx(ctx, tx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		return ErrBucketNotFound
	}
	return nil
}

func (s *SQLMetadata) bucketExistsInTx(ctx context.Context, tx *dbx.Tx, bucket string) (bool, error) {
	query := querydsl.SelectValue(metadataBuckets.name).
		From(metadataBuckets.schema).
		Where(metadataBuckets.name.Eq(bucket)).
		Limit(1)
	option, err := dbx.QueryScalarOption(ensureContext(ctx), tx, query)
	if err != nil {
		return false, fmt.Errorf("query bucket exists: %w", err)
	}
	return option.IsPresent(), nil
}

func (s *SQLMetadata) deleteBucketObjectsInTx(ctx context.Context, tx *dbx.Tx, bucket, state string) error {
	query := querydsl.DeleteFrom(metadataObjects.schema).
		Where(querydsl.And(metadataObjects.bucket.Eq(bucket), metadataObjects.state.Eq(state)))
	if _, err := dbx.Exec(ensureContext(ctx), tx, query); err != nil {
		return fmt.Errorf("delete bucket objects: %w", err)
	}
	return nil
}

func (s *SQLMetadata) deleteBucketRowInTx(ctx context.Context, tx *dbx.Tx, bucket string) error {
	query := querydsl.DeleteFrom(metadataBuckets.schema).
		Where(metadataBuckets.name.Eq(bucket))
	if _, err := dbx.Exec(ensureContext(ctx), tx, query); err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	return nil
}
