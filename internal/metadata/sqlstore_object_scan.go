package metadata

import (
	"database/sql"
	"fmt"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func scanObjectMeta(scanner interface{ Scan(dest ...any) error }) (model.ObjectMeta, error) {
	var (
		meta         model.ObjectMeta
		userMetadata sql.NullString
		intentID     sql.NullString
		intentStage  sql.NullString
		intentStart  sql.NullInt64
		intentUpdate sql.NullInt64
		placements   sql.NullString
		checksums    sql.NullString
		shardSizes   sql.NullString
		updatedAt    int64
	)
	if err := scanner.Scan(
		&meta.Bucket,
		&meta.Key,
		&meta.Hash,
		&meta.ETag,
		&meta.Size,
		&meta.ContentType,
		&meta.CacheControl,
		&meta.ContentDisposition,
		&meta.ContentEncoding,
		&meta.ContentLanguage,
		&userMetadata,
		&updatedAt,
		&meta.State,
		&intentID,
		&intentStage,
		&intentStart,
		&intentUpdate,
		&placements,
		&checksums,
		&shardSizes,
	); err != nil {
		return model.ObjectMeta{}, fmt.Errorf("scan object meta: %w", err)
	}

	if err := hydrateObjectMeta(&meta, updatedAt, userMetadata, placements, checksums, shardSizes); err != nil {
		return model.ObjectMeta{}, err
	}
	if intentID.Valid {
		meta.WriteIntent = &model.WriteIntent{
			ID:        intentID.String,
			Stage:     emptyStringOrDefault(intentStage),
			StartedAt: unixNanoToTimeOpt(intentStart),
			UpdatedAt: unixNanoToTimeOpt(intentUpdate),
		}
	}
	return meta, nil
}

func hydrateObjectMeta(
	meta *model.ObjectMeta,
	updatedAt int64,
	userMetadata sql.NullString,
	placements sql.NullString,
	checksums sql.NullString,
	shardSizes sql.NullString,
) error {
	meta.UpdatedAt = unixNanoToTime(updatedAt)
	if err := decodeJSON(userMetadata, &meta.UserMetadata); err != nil {
		return fmt.Errorf("decode user metadata: %w", err)
	}
	if err := decodeJSON(placements, &meta.ShardPlacements); err != nil {
		return fmt.Errorf("decode shard placements: %w", err)
	}
	if err := decodeJSON(checksums, &meta.ShardChecksums); err != nil {
		return fmt.Errorf("decode shard checksums: %w", err)
	}
	if err := decodeJSON(shardSizes, &meta.ShardSizes); err != nil {
		return fmt.Errorf("decode shard sizes: %w", err)
	}
	return nil
}
