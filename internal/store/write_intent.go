package store

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/model"
)

func newWriteIntent(bucket, key, hash string, at time.Time) *model.WriteIntent {
	return &model.WriteIntent{
		ID:        strings.Join([]string{bucket, key, hash, strconv.FormatInt(at.UnixNano(), 10)}, ":"),
		Stage:     model.WriteIntentStageMetadataStaged,
		StartedAt: at,
		UpdatedAt: at,
	}
}

func advanceWriteIntent(intent *model.WriteIntent, stage string, at time.Time) *model.WriteIntent {
	if intent == nil {
		return &model.WriteIntent{
			ID:        strconv.FormatInt(at.UnixNano(), 10),
			Stage:     stage,
			StartedAt: at,
			UpdatedAt: at,
		}
	}
	next := *intent
	next.Stage = stage
	next.UpdatedAt = at
	return &next
}

func (s *Store) updatePendingWriteIntent(
	ctx context.Context,
	meta model.ObjectMeta,
	stage string,
) (model.ObjectMeta, error) {
	meta.WriteIntent = advanceWriteIntent(meta.WriteIntent, stage, time.Now().UTC())
	meta.UpdatedAt = meta.WriteIntent.UpdatedAt
	if err := s.meta.StageObjectMeta(ctx, meta); err != nil {
		return model.ObjectMeta{}, fmt.Errorf("update pending write intent: %w", mapStoreError(err))
	}
	return meta, nil
}

func writeIntentStage(meta model.ObjectMeta) string {
	if meta.WriteIntent == nil || strings.TrimSpace(meta.WriteIntent.Stage) == "" {
		return model.WriteIntentStageUnknown
	}
	return meta.WriteIntent.Stage
}
