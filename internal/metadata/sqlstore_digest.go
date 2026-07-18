package metadata

import (
	"context"
	"fmt"
	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
	"github.com/lyonbrown4d/maxio/internal/model"
	"strings"
	"time"
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
	if _, execErr := dbx.Exec(ensureContext(ctx), s.dbxDB, query); execErr != nil {
		return model.DigestRef{}, fmt.Errorf("upsert digest ref: %w", execErr)
	}
	return requireStoredEntity(s.GetDigestRef(ctx, ref.Digest))
}

func (s *SQLMetadata) GetDigestRef(ctx context.Context, digest string) (model.DigestRef, bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return model.DigestRef{}, false, ErrBadRequest
	}

	return getRepositoryByKey[model.DigestRef](
		ctx,
		s.repos.digestRefs,
		repositoryx.KeySet(repositoryx.Part(metadataDigestRefs.digest, digest)),
		"query digest ref",
	)
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
	err := s.withTx(ctx, "release digest ref", func(tx *dbx.Tx) error {
		var queryErr error
		ref, queryErr = s.getDigestRefInTx(ctx, tx, digest)
		if queryErr != nil {
			return queryErr
		}
		ref.RefCount--
		ref.UpdatedAt = time.Now().UTC()
		if ref.RefCount <= 0 {
			removed = true
			return s.deleteDigestRefInTx(ctx, tx, digest)
		}
		return s.updateDigestRefInTx(ctx, tx, ref)
	})
	return ref, removed, err
}

func (s *SQLMetadata) DeleteDigestRef(ctx context.Context, digest string) (bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return false, ErrBadRequest
	}
	result, err := s.repos.digestRefs.DeleteByKeySet(ctx, repositoryx.KeySet(repositoryx.Part(metadataDigestRefs.digest, digest)))
	if err != nil {
		return false, fmt.Errorf("delete digest ref: %w", err)
	}
	return hasAffectedRow(result, "delete digest ref")
}

func (s *SQLMetadata) getDigestRefInTx(ctx context.Context, tx *dbx.Tx, digest string) (model.DigestRef, error) {
	query := querydsl.SelectFrom(metadataDigestRefs.schema, metadataDigestRefs.selectItems()...).
		Where(metadataDigestRefs.digest.Eq(digest)).
		Limit(1)
	option, err := dbx.QueryOption(ensureContext(ctx), tx, query, s.repos.digestRefs.Mapper())
	if err != nil {
		return model.DigestRef{}, fmt.Errorf("query digest ref: %w", err)
	}
	ref, found := option.Get()
	if !found {
		return model.DigestRef{}, ErrObjectNotFound
	}
	return ref, nil
}

func (s *SQLMetadata) deleteDigestRefInTx(ctx context.Context, tx *dbx.Tx, digest string) error {
	query := querydsl.DeleteFrom(metadataDigestRefs.schema).
		Where(metadataDigestRefs.digest.Eq(digest))
	if _, err := dbx.Exec(ensureContext(ctx), tx, query); err != nil {
		return fmt.Errorf("delete digest ref: %w", err)
	}
	return nil
}

func (s *SQLMetadata) updateDigestRefInTx(ctx context.Context, tx *dbx.Tx, ref model.DigestRef) error {
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
	if _, err := dbx.Exec(ensureContext(ctx), tx, query); err != nil {
		return fmt.Errorf("update digest ref: %w", err)
	}
	return nil
}
