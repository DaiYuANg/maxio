package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/model"
)

const (
	PendingRecoveryActionWait           = "wait"
	PendingRecoveryActionDeleteStaged   = "delete_staged"
	PendingRecoveryActionRollbackLayout = "rollback_layout"
	PendingRecoveryActionReleaseBlob    = "release_blob"
	PendingRecoveryActionCommitted      = "committed_cleanup"
)

type PendingRecoveryAction struct {
	Bucket  string `json:"bucket"`
	Key     string `json:"key"`
	Hash    string `json:"hash"`
	Stage   string `json:"stage"`
	Action  string `json:"action"`
	Expired bool   `json:"expired"`
	Reason  string `json:"reason,omitempty"`
}

func (s *Store) rollbackExpiredPendingWrite(
	ctx context.Context,
	meta model.ObjectMeta,
	logger *slog.Logger,
) error {
	action := pendingRecoveryAction(meta, true)
	switch action.Action {
	case PendingRecoveryActionRollbackLayout:
		return s.rollbackPendingLayout(ctx, meta, logger)
	case PendingRecoveryActionReleaseBlob:
		return s.rollbackPendingLayoutAndBlob(ctx, meta, logger)
	default:
		return nil
	}
}

func (s *Store) rollbackPendingLayout(ctx context.Context, meta model.ObjectMeta, logger *slog.Logger) error {
	restored, err := s.restoreCommittedLayout(ctx, meta)
	if err != nil {
		return err
	}
	if restored {
		logPendingRollback(ctx, logger, meta, PendingRecoveryActionRollbackLayout)
		return nil
	}
	if err := s.engine.DeleteObjectLayout(ctx, meta.Bucket, meta.Key); err != nil && !errors.Is(err, engine.ErrObjectNotFound) {
		return fmt.Errorf("delete pending object layout: %w", mapStoreError(err))
	}
	logPendingRollback(ctx, logger, meta, PendingRecoveryActionRollbackLayout)
	return nil
}

func (s *Store) restoreCommittedLayout(ctx context.Context, pending model.ObjectMeta) (bool, error) {
	committed, exists, err := s.meta.GetObjectMeta(ctx, pending.Bucket, pending.Key)
	if err != nil {
		return false, fmt.Errorf("get committed object for pending layout rollback: %w", mapStoreError(err))
	}
	if !exists {
		return false, nil
	}
	ref, exists, err := s.meta.GetBlobRef(ctx, committed.Hash)
	if err != nil {
		return false, fmt.Errorf("get committed blob ref for pending layout rollback: %w", mapStoreError(err))
	}
	if !exists {
		return false, fmt.Errorf("committed blob ref %s missing: %w", committed.Hash, ErrNotFound)
	}
	_, err = s.engine.LinkObject(ctx, committed.Bucket, committed.Key, engine.BlobInfo{
		Hash:            committed.Hash,
		ETag:            committed.ETag,
		Size:            ref.Size,
		ShardDir:        ref.Path,
		ShardPlacements: ref.ShardPlacements,
		ShardChecksums:  ref.ShardChecksums,
		ShardSizes:      ref.ShardSizes,
	}, committed.ContentType, committed.UpdatedAt)
	if err != nil {
		return false, fmt.Errorf("restore committed object layout: %w", mapStoreError(err))
	}
	return true, nil
}

func (s *Store) rollbackPendingLayoutAndBlob(ctx context.Context, meta model.ObjectMeta, logger *slog.Logger) error {
	if err := s.rollbackPendingLayout(ctx, meta, logger); err != nil {
		return err
	}
	shouldRelease, err := s.shouldReleasePendingBlob(ctx, meta)
	if err != nil {
		return err
	}
	if !shouldRelease {
		return nil
	}
	if err := s.releaseBlob(ctx, meta.Hash); err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("release pending blob: %w", err)
	}
	logPendingRollback(ctx, logger, meta, PendingRecoveryActionReleaseBlob)
	return nil
}

func (s *Store) shouldReleasePendingBlob(ctx context.Context, meta model.ObjectMeta) (bool, error) {
	committed, exists, err := s.meta.GetObjectMeta(ctx, meta.Bucket, meta.Key)
	if err != nil {
		return false, fmt.Errorf("get committed object for pending rollback: %w", mapStoreError(err))
	}
	return !exists || committed.Hash != meta.Hash, nil
}

func pendingRecoveryAction(meta model.ObjectMeta, expired bool) PendingRecoveryAction {
	action := PendingRecoveryAction{
		Bucket:  meta.Bucket,
		Key:     meta.Key,
		Hash:    meta.Hash,
		Stage:   writeIntentStage(meta),
		Expired: expired,
	}
	if !expired {
		action.Action = PendingRecoveryActionWait
		return action
	}
	action.Action, action.Reason = pendingExpiredRecoveryDecision(action.Stage)
	return action
}

func pendingExpiredRecoveryDecision(stage string) (string, string) {
	switch stage {
	case model.WriteIntentStageCommitted:
		return PendingRecoveryActionCommitted, "committed metadata already owns the object; only staged metadata is stale"
	case model.WriteIntentStageBlobRetained:
		return PendingRecoveryActionReleaseBlob, "blob ref was retained but object was not committed"
	case model.WriteIntentStageLayoutLinked:
		return PendingRecoveryActionRollbackLayout, "layout was linked before metadata commit"
	default:
		return PendingRecoveryActionDeleteStaged, "pending metadata expired before commit"
	}
}

func logPendingRollback(ctx context.Context, logger *slog.Logger, meta model.ObjectMeta, action string) {
	if logger == nil {
		return
	}
	logger.WarnContext(ctx, "pending write rolled back",
		"bucket", meta.Bucket,
		"key", meta.Key,
		"hash", meta.Hash,
		"stage", writeIntentStage(meta),
		"action", action,
	)
}
