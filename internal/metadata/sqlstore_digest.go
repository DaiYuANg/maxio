package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/arcgolabs/dbx/querydsl"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) UpsertDigestRef(ctx context.Context, ref model.DigestRef) (model.DigestRef, error) {
	ref, err := prepareDBDigestRef(ref)
	if err != nil {
		return model.DigestRef{}, err
	}
	assignments, err := s.repos.digestRefs.Mapper().InsertAssignmentsWithID(ctx, metadataDigestRefs.schema, &ref, nil)
	if err != nil {
		return model.DigestRef{}, fmt.Errorf("map digest ref insert assignments: %w", err)
	}
	query := querydsl.InsertInto(metadataDigestRefs.schema).
		ValuesList(assignments).
		OnConflict(metadataDigestRefs.digest).
		DoUpdateSet(
			metadataDigestRefs.size.SetExcluded(),
			metadataDigestRefs.refCount.SetExcluded(),
			metadataDigestRefs.upstreamID.SetExcluded(),
			metadataDigestRefs.upstreamBucket.SetExcluded(),
			metadataDigestRefs.upstreamKey.SetExcluded(),
			metadataDigestRefs.updatedAt.SetExcluded(),
		)
	if _, execErr := s.execBuilderContext(ctx, query); execErr != nil {
		return model.DigestRef{}, fmt.Errorf("upsert digest ref: %w", execErr)
	}
	stored, found, err := s.GetDigestRef(ctx, ref.Digest)
	if err != nil {
		return model.DigestRef{}, err
	}
	if !found {
		return model.DigestRef{}, ErrObjectNotFound
	}
	return stored, nil
}

func (s *SQLMetadata) GetDigestRef(ctx context.Context, digest string) (model.DigestRef, bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return model.DigestRef{}, false, ErrBadRequest
	}

	query := querydsl.SelectFrom(metadataDigestRefs.schema, metadataDigestRefs.selectItems()...).
		Where(metadataDigestRefs.digest.Eq(digest)).
		Limit(1)
	option, err := s.repos.digestRefs.FirstOption(ctx, query)
	if err != nil {
		return model.DigestRef{}, false, fmt.Errorf("query digest ref: %w", err)
	}
	ref, found := option.Get()
	return ref, found, nil
}

func (s *SQLMetadata) RetainDigestRef(ctx context.Context, ref model.DigestRef) (model.DigestRef, error) {
	ref, err := prepareDBDigestRef(ref)
	if err != nil {
		return model.DigestRef{}, err
	}
	existing, found, err := s.GetDigestRef(ctx, ref.Digest)
	if err != nil {
		return model.DigestRef{}, err
	}
	if found {
		ref.CreatedAt = existing.CreatedAt
		ref.RefCount = existing.RefCount + 1
		ref.UpstreamID = existing.UpstreamID
		ref.UpstreamBucket = existing.UpstreamBucket
		ref.UpstreamKey = existing.UpstreamKey
	}
	return s.UpsertDigestRef(ctx, ref)
}

func (s *SQLMetadata) ReleaseDigestRef(ctx context.Context, digest string) (model.DigestRef, bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return model.DigestRef{}, false, ErrBadRequest
	}

	var ref model.DigestRef
	var removed bool
	err := s.withTx(ctx, "release digest ref", func(tx *sql.Tx) error {
		var err error
		ref, err = s.getDigestRefInTx(ctx, tx, digest)
		if err != nil {
			return err
		}
		ref.RefCount--
		ref.UpdatedAt = time.Now().UTC()
		if ref.RefCount <= 0 {
			removed = true
			query := querydsl.DeleteFrom(metadataDigestRefs.schema).
				Where(metadataDigestRefs.digest.Eq(digest))
			return s.txExecBuilderContext(ctx, tx, query)
		}
		assignments, err := s.repos.digestRefs.Mapper().UpdateAssignments(metadataDigestRefs.schema, &ref)
		if err != nil {
			return fmt.Errorf("map digest ref update assignments: %w", err)
		}
		predicate, err := s.repos.digestRefs.Mapper().PrimaryPredicate(metadataDigestRefs.schema, &ref)
		if err != nil {
			return fmt.Errorf("map digest ref primary predicate: %w", err)
		}
		query := querydsl.Update(metadataDigestRefs.schema).
			SetList(assignments).
			Where(predicate)
		return s.txExecBuilderContext(ctx, tx, query)
	})
	return ref, removed, err
}

func (s *SQLMetadata) DeleteDigestRef(ctx context.Context, digest string) (bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return false, ErrBadRequest
	}
	query := querydsl.DeleteFrom(metadataDigestRefs.schema).
		Where(metadataDigestRefs.digest.Eq(digest))
	result, err := s.repos.digestRefs.Delete(ctx, query)
	if err != nil {
		return false, fmt.Errorf("delete digest ref: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete digest ref rows: %w", err)
	}
	return affected > 0, nil
}

func (s *SQLMetadata) getDigestRefInTx(ctx context.Context, tx *sql.Tx, digest string) (model.DigestRef, error) {
	query := querydsl.SelectFrom(metadataDigestRefs.schema, metadataDigestRefs.selectItems()...).
		Where(metadataDigestRefs.digest.Eq(digest)).
		Limit(1)
	ref, found, err := querySQLOneInTx(ctx, s, tx, query, "digest ref", s.repos.digestRefs.Mapper())
	if err != nil {
		return model.DigestRef{}, err
	}
	if !found {
		return model.DigestRef{}, ErrObjectNotFound
	}
	return ref, nil
}
