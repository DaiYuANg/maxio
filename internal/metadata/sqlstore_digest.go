package metadata

import (
	"context"
	"fmt"

	collectionlist "github.com/arcgolabs/collectionx/list"
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
	if err := execRepositoryUpsert(
		ctx,
		s.repos.digestRefs,
		metadataDigestRefs.schema,
		&ref,
		"map digest ref insert assignments",
		"upsert digest ref",
		collectionlist.NewList[querydsl.Expression](metadataDigestRefs.digest),
		metadataDigestRefs.size.SetExcluded(),
		metadataDigestRefs.refCount.SetExcluded(),
		metadataDigestRefs.upstreamID.SetExcluded(),
		metadataDigestRefs.upstreamBucket.SetExcluded(),
		metadataDigestRefs.upstreamKey.SetExcluded(),
		metadataDigestRefs.updatedAt.SetExcluded(),
	); err != nil {
		return model.DigestRef{}, err
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
	ctx = ensureContext(ctx)
	err := s.repos.digestRefs.InTx(ctx, nil, func(_ *dbx.Tx, repo *repositoryx.Base[model.DigestRef, metadataDigestRefsSchema]) error {
		var queryErr error
		ref, queryErr = s.getDigestRefInTx(ctx, repo, digest)
		if queryErr != nil {
			return queryErr
		}
		ref.RefCount--
		ref.UpdatedAt = time.Now().UTC()
		if ref.RefCount <= 0 {
			removed = true
			return s.deleteDigestRefInTx(ctx, repo, digest)
		}
		return s.updateDigestRefInTx(ctx, repo, ref)
	})
	if err != nil {
		return ref, removed, fmt.Errorf("release digest ref: %w", err)
	}
	return ref, removed, nil
}

func (s *SQLMetadata) DeleteDigestRef(ctx context.Context, digest string) (bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return false, ErrBadRequest
	}
	return deleteRepositoryByKey[model.DigestRef](
		ctx,
		s.repos.digestRefs,
		repositoryx.KeySet(repositoryx.Part(metadataDigestRefs.digest, digest)),
		"delete digest ref",
	)
}

func (s *SQLMetadata) getDigestRefInTx(ctx context.Context, repo *repositoryx.Base[model.DigestRef, metadataDigestRefsSchema], digest string) (model.DigestRef, error) {
	option, err := repo.GetByKeySetOption(ctx, repositoryx.KeySet(repositoryx.Part(metadataDigestRefs.digest, digest)))
	if err != nil {
		return model.DigestRef{}, fmt.Errorf("query digest ref: %w", err)
	}
	ref, found := option.Get()
	if !found {
		return model.DigestRef{}, ErrObjectNotFound
	}
	return ref, nil
}

func (s *SQLMetadata) deleteDigestRefInTx(ctx context.Context, repo *repositoryx.Base[model.DigestRef, metadataDigestRefsSchema], digest string) error {
	if _, err := repo.DeleteByKeySet(ctx, repositoryx.KeySet(repositoryx.Part(metadataDigestRefs.digest, digest))); err != nil {
		return fmt.Errorf("delete digest ref: %w", err)
	}
	return nil
}

func (s *SQLMetadata) updateDigestRefInTx(ctx context.Context, repo *repositoryx.Base[model.DigestRef, metadataDigestRefsSchema], ref model.DigestRef) error {
	assignments, err := repo.Mapper().UpdateAssignments(metadataDigestRefs.schema, &ref)
	if err != nil {
		return fmt.Errorf("map digest ref update assignments: %w", err)
	}
	if _, err := repo.UpdateByKeySet(ctx, repositoryx.KeySet(repositoryx.Part(metadataDigestRefs.digest, ref.Digest)), assignments.Values()...); err != nil {
		return fmt.Errorf("update digest ref: %w", err)
	}
	return nil
}
