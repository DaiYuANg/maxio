package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SQLMetadata) UpsertDigestRef(ctx context.Context, ref model.DigestRef) (model.DigestRef, error) {
	ref, err := prepareDBDigestRef(ref)
	if err != nil {
		return model.DigestRef{}, err
	}
	if _, execErr := s.execContext(
		ctx,
		sqlStoreDigestRefUpsertSQL,
		ref.Digest,
		ref.Size,
		ref.RefCount,
		ref.UpstreamID,
		ref.UpstreamBucket,
		ref.UpstreamKey,
		ref.CreatedAt.UnixNano(),
		ref.UpdatedAt.UnixNano(),
	); execErr != nil {
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

	row := s.queryRowContext(
		ctx,
		`SELECT `+sqlStoreDigestRefColumns+`
		   FROM metadata_digest_refs
		  WHERE digest = ?
		  LIMIT 1`,
		digest,
	)
	ref, err := scanDigestRef(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.DigestRef{}, false, nil
	}
	if err != nil {
		return model.DigestRef{}, false, fmt.Errorf("get digest ref: %w", err)
	}
	return ref, true, nil
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
			return s.txExecContext(ctx, tx, `DELETE FROM metadata_digest_refs WHERE digest = ?`, digest)
		}
		return s.txExecContext(
			ctx,
			tx,
			`UPDATE metadata_digest_refs
			    SET ref_count = ?, updated_at = ?
			  WHERE digest = ?`,
			ref.RefCount,
			ref.UpdatedAt.UnixNano(),
			digest,
		)
	})
	return ref, removed, err
}

func (s *SQLMetadata) DeleteDigestRef(ctx context.Context, digest string) (bool, error) {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return false, ErrBadRequest
	}
	result, err := s.execContext(ctx, `DELETE FROM metadata_digest_refs WHERE digest = ?`, digest)
	if err != nil {
		return false, fmt.Errorf("delete digest ref: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete digest ref rows: %w", err)
	}
	return affected > 0, nil
}
