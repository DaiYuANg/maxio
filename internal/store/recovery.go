package store

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/model"
)

type RecoveryOptions struct {
	PendingTTL          time.Duration
	CleanupOrphanShards bool
	DryRun              bool
	Logger              *slog.Logger
}

type RecoveryResult struct {
	StartedAt          time.Time                       `json:"started_at"`
	FinishedAt         time.Time                       `json:"finished_at"`
	PendingRemoved     int                             `json:"pending_removed"`
	PendingActions     map[string]int                  `json:"pending_actions,omitempty"`
	OrphanShardCleanup engine.OrphanShardCleanupResult `json:"orphan_shard_cleanup"`
	DryRun             bool                            `json:"dry_run"`
}

type RecoveryStatus struct {
	LastResult RecoveryResult `json:"last_result"`
	LastError  string         `json:"last_error,omitempty"`
	Completed  bool           `json:"completed"`
}

type RecoveryPlan struct {
	GeneratedAt           time.Time                       `json:"generated_at"`
	PendingObjects        []model.ObjectMeta              `json:"pending_objects,omitempty"`
	ExpiredPendingObjects []model.ObjectMeta              `json:"expired_pending_objects,omitempty"`
	PendingActions        []PendingRecoveryAction         `json:"pending_actions,omitempty"`
	WriteIntentStages     map[string]int                  `json:"write_intent_stages,omitempty"`
	OrphanShardCleanup    engine.OrphanShardCleanupResult `json:"orphan_shard_cleanup"`
}

type recoveryState struct {
	mu        sync.RWMutex
	result    RecoveryResult
	lastError string
	completed bool
}

func (s *Store) Recover(ctx context.Context, opts RecoveryOptions) (RecoveryResult, error) {
	result := RecoveryResult{
		StartedAt: time.Now().UTC(),
		DryRun:    opts.DryRun,
	}
	pending, err := s.cleanupPendingObjects(ctx, opts.PendingTTL, opts.Logger)
	result.PendingRemoved = pending.removed
	result.PendingActions = pending.actions
	if err != nil {
		result.FinishedAt = time.Now().UTC()
		s.setRecoveryStatus(result, err)
		return result, err
	}
	if opts.CleanupOrphanShards {
		cleanup, cleanupErr := s.cleanupOrphanShardSets(ctx, opts.DryRun)
		result.OrphanShardCleanup = cleanup
		if cleanupErr != nil {
			result.FinishedAt = time.Now().UTC()
			s.setRecoveryStatus(result, cleanupErr)
			return result, cleanupErr
		}
	}
	result.FinishedAt = time.Now().UTC()
	s.setRecoveryStatus(result, nil)
	return result, nil
}

func (s *Store) PlanRecovery(ctx context.Context, pendingTTL time.Duration) (RecoveryPlan, error) {
	plan := RecoveryPlan{GeneratedAt: time.Now().UTC()}
	pending, err := s.meta.ListStagedObjectMetas(ctx, "", "")
	if err != nil {
		return plan, fmt.Errorf("list staged object metadata: %w", mapStoreError(err))
	}
	plan.PendingObjects = pending
	plan.ExpiredPendingObjects = expiredPendingObjects(pending, pendingTTL, plan.GeneratedAt)
	plan.PendingActions = pendingRecoveryActions(pending, pendingTTL, plan.GeneratedAt)
	plan.WriteIntentStages = writeIntentStageCounts(pending)
	orphanCleanup, err := s.cleanupOrphanShardSets(ctx, true)
	if err != nil {
		return plan, err
	}
	plan.OrphanShardCleanup = orphanCleanup
	return plan, nil
}

func pendingRecoveryActions(objects []model.ObjectMeta, ttl time.Duration, now time.Time) []PendingRecoveryAction {
	if len(objects) == 0 {
		return nil
	}
	cutoff := now.UTC().Add(-ttl)
	actions := make([]PendingRecoveryAction, 0, len(objects))
	for index := range objects {
		expired := ttl > 0 && isExpiredPendingObject(objects[index], cutoff)
		actions = append(actions, pendingRecoveryAction(objects[index], expired))
	}
	return actions
}

func writeIntentStageCounts(objects []model.ObjectMeta) map[string]int {
	counts := make(map[string]int)
	for index := range objects {
		counts[writeIntentStage(objects[index])]++
	}
	if len(counts) == 0 {
		return nil
	}
	return counts
}

func (s *Store) RecoveryStatus() RecoveryStatus {
	if s == nil {
		return RecoveryStatus{}
	}
	s.recovery.mu.RLock()
	defer s.recovery.mu.RUnlock()
	return RecoveryStatus{
		LastResult: s.recovery.result,
		LastError:  s.recovery.lastError,
		Completed:  s.recovery.completed,
	}
}

func (s *Store) setRecoveryStatus(result RecoveryResult, err error) {
	if s == nil {
		return
	}
	s.recovery.mu.Lock()
	defer s.recovery.mu.Unlock()
	s.recovery.result = result
	s.recovery.completed = true
	if err != nil {
		s.recovery.lastError = err.Error()
		return
	}
	s.recovery.lastError = ""
}

func (s *Store) cleanupOrphanShardSets(ctx context.Context, dryRun bool) (engine.OrphanShardCleanupResult, error) {
	refs, err := s.meta.ListBlobRefs(ctx)
	if err != nil {
		return engine.OrphanShardCleanupResult{}, fmt.Errorf("list blob refs: %w", mapStoreError(err))
	}
	live := make([]engine.ShardSetRef, 0, len(refs))
	for index := range refs {
		ref := refs[index]
		live = append(live, engine.ShardSetRef{ShardDir: ref.Path, Hash: ref.Hash})
	}
	pending, err := s.meta.ListStagedObjectMetas(ctx, "", "")
	if err != nil {
		return engine.OrphanShardCleanupResult{}, fmt.Errorf("list staged object metadata: %w", mapStoreError(err))
	}
	live = append(live, pendingShardSetRefs(pending)...)
	result, err := s.engine.CleanupOrphanShardSets(ctx, live, dryRun)
	if err != nil {
		return result, fmt.Errorf("cleanup orphan shard sets: %w", mapStoreError(err))
	}
	return result, nil
}

func pendingShardSetRefs(objects []model.ObjectMeta) []engine.ShardSetRef {
	if len(objects) == 0 {
		return nil
	}
	refs := make([]engine.ShardSetRef, 0, len(objects))
	for index := range objects {
		meta := objects[index]
		if strings.TrimSpace(meta.Key) == "" || strings.TrimSpace(meta.Hash) == "" {
			continue
		}
		refs = append(refs, engine.ShardSetRef{
			ShardDir: engine.ShardDirForKey(meta.Key),
			Hash:     meta.Hash,
		})
	}
	return refs
}

func expiredPendingObjects(objects []model.ObjectMeta, ttl time.Duration, now time.Time) []model.ObjectMeta {
	if ttl <= 0 {
		return nil
	}
	cutoff := now.UTC().Add(-ttl)
	expired := make([]model.ObjectMeta, 0)
	for index := range objects {
		if isExpiredPendingObject(objects[index], cutoff) {
			expired = append(expired, objects[index])
		}
	}
	return expired
}
