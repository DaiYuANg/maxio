package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/arcgolabs/dbx/querydsl"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func scanObjectRecord(scanner interface{ Scan(dest ...any) error }) (model.ObjectRecord, error) {
	var (
		record  model.ObjectRecord
		deleted int
		created int64
		updated int64
	)
	if err := scanner.Scan(
		&record.Bucket,
		&record.Key,
		&record.CurrentVersionID,
		&deleted,
		&created,
		&updated,
	); err != nil {
		return model.ObjectRecord{}, fmt.Errorf("scan object record: %w", err)
	}
	record.Deleted = intToBool(deleted)
	record.CreatedAt = unixNanoToTime(created)
	record.UpdatedAt = unixNanoToTime(updated)
	return record, nil
}

func scanObjectVersion(scanner interface{ Scan(dest ...any) error }) (model.ObjectVersion, error) {
	var (
		version      model.ObjectVersion
		userMetadata sql.NullString
		deleteMarker int
		created      int64
		updated      int64
	)
	if err := scanner.Scan(
		&version.Bucket,
		&version.Key,
		&version.VersionID,
		&version.Digest,
		&version.ETag,
		&version.Size,
		&version.ContentType,
		&version.CacheControl,
		&version.ContentDisposition,
		&version.ContentEncoding,
		&version.ContentLanguage,
		&userMetadata,
		&version.UpstreamID,
		&version.UpstreamBucket,
		&version.UpstreamKey,
		&deleteMarker,
		&created,
		&updated,
	); err != nil {
		return model.ObjectVersion{}, fmt.Errorf("scan object version: %w", err)
	}
	if err := decodeJSON(userMetadata, &version.UserMetadata); err != nil {
		return model.ObjectVersion{}, fmt.Errorf("decode object version user metadata: %w", err)
	}
	version.DeleteMarker = intToBool(deleteMarker)
	version.CreatedAt = unixNanoToTime(created)
	version.UpdatedAt = unixNanoToTime(updated)
	return version, nil
}

func scanDigestRef(scanner interface{ Scan(dest ...any) error }) (model.DigestRef, error) {
	var (
		ref     model.DigestRef
		created int64
		updated int64
	)
	if err := scanner.Scan(
		&ref.Digest,
		&ref.Size,
		&ref.RefCount,
		&ref.UpstreamID,
		&ref.UpstreamBucket,
		&ref.UpstreamKey,
		&created,
		&updated,
	); err != nil {
		return model.DigestRef{}, fmt.Errorf("scan digest ref: %w", err)
	}
	ref.CreatedAt = unixNanoToTime(created)
	ref.UpdatedAt = unixNanoToTime(updated)
	return ref, nil
}

func (s *SQLMetadata) getDigestRefInTx(ctx context.Context, tx *sql.Tx, digest string) (model.DigestRef, error) {
	query := querydsl.SelectFrom(metadataDigestRefs.table, metadataDigestRefs.selectItems()...).
		Where(metadataDigestRefs.digest.Eq(digest)).
		Limit(1)
	row, queryErr := s.txQueryRowBuilderContext(ctx, tx, query)
	if queryErr != nil {
		return model.DigestRef{}, fmt.Errorf("get digest ref: %w", queryErr)
	}
	ref, err := scanDigestRef(row)
	if errors.Is(err, sql.ErrNoRows) {
		return model.DigestRef{}, ErrObjectNotFound
	}
	if err != nil {
		return model.DigestRef{}, fmt.Errorf("get digest ref: %w", err)
	}
	return ref, nil
}
