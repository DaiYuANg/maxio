package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
)

type restartRecoveryScenario struct {
	dataDir         string
	meta            *metadata.InMemoryMetadata
	original        []byte
	committed       model.ObjectMeta
	freshBlob       engine.BlobInfo
	replacementBlob engine.BlobInfo
	retainedBlob    engine.BlobInfo
}

func TestStoreRestartRecoveryPlansAndRecoversPendingWrites(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	scenario := newRestartRecoveryScenario(ctx, t)
	restartedStore, restartedEngine := newRestartedRecoveryStore(t, scenario.dataDir, scenario.meta)

	plan, err := restartedStore.PlanRecovery(ctx, time.Hour)
	mustNoError(t, err, "plan recovery after restart")
	assertRestartRecoveryPlan(t, plan)
	assertRestartRecoveryPlanDidNotMutate(ctx, t, scenario, restartedEngine)

	result, err := restartedStore.Recover(ctx, store.RecoveryOptions{
		PendingTTL:          time.Hour,
		CleanupOrphanShards: true,
	})
	mustNoError(t, err, "recover after restart")
	assertRestartRecoveryResult(t, result)
	assertRestartRecoveryPostState(ctx, t, scenario, restartedEngine)
	assertRecoveredObject(ctx, t, restartedStore, scenario.original, scenario.committed.Hash)
}

func assertRestartRecoveryPlan(t *testing.T, plan store.RecoveryPlan) {
	t.Helper()

	if len(plan.PendingObjects) != 4 {
		t.Fatalf("planned pending objects = %d, want 4", len(plan.PendingObjects))
	}
	if len(plan.ExpiredPendingObjects) != 3 {
		t.Fatalf("planned expired pending objects = %d, want 3", len(plan.ExpiredPendingObjects))
	}
	if plan.OrphanShardCleanup.Removed != 0 || len(plan.OrphanShardCleanup.Orphans) != 0 {
		t.Fatalf("planned orphan cleanup = %+v, want no removals or orphans", plan.OrphanShardCleanup)
	}
	assertRestartRecoveryCounts(t, plan.WriteIntentStages, map[string]int{
		model.WriteIntentStageBlobPrepared:   1,
		model.WriteIntentStageMetadataStaged: 1,
		model.WriteIntentStageLayoutLinked:   1,
		model.WriteIntentStageBlobRetained:   1,
	}, "planned write intent stages")
	assertRestartRecoveryCounts(t, restartRecoveryActionCounts(plan.PendingActions), map[string]int{
		store.PendingRecoveryActionWait:           1,
		store.PendingRecoveryActionDeleteStaged:   1,
		store.PendingRecoveryActionRollbackLayout: 1,
		store.PendingRecoveryActionReleaseBlob:    1,
	}, "planned pending actions")
}

func assertRestartRecoveryPlanDidNotMutate(
	ctx context.Context,
	t *testing.T,
	scenario restartRecoveryScenario,
	eng *engine.Engine,
) {
	t.Helper()

	staged, err := scenario.meta.ListStagedObjectMetas(ctx, "", "")
	mustNoError(t, err, "list staged objects after plan")
	if len(staged) != 4 {
		t.Fatalf("staged objects after plan = %d, want 4", len(staged))
	}
	assertRestartShardExists(ctx, t, eng, scenario.freshBlob, "fresh pending shard was removed by recovery plan")
}

func assertRestartRecoveryResult(t *testing.T, result store.RecoveryResult) {
	t.Helper()

	if result.PendingRemoved != 3 {
		t.Fatalf("pending removed = %d, want 3", result.PendingRemoved)
	}
	assertRestartRecoveryCounts(t, result.PendingActions, map[string]int{
		store.PendingRecoveryActionDeleteStaged:   1,
		store.PendingRecoveryActionRollbackLayout: 1,
		store.PendingRecoveryActionReleaseBlob:    1,
	}, "recovery pending actions")
}

func assertRestartRecoveryPostState(
	ctx context.Context,
	t *testing.T,
	scenario restartRecoveryScenario,
	eng *engine.Engine,
) {
	t.Helper()

	staged, err := scenario.meta.ListStagedObjectMetas(ctx, "", "")
	mustNoError(t, err, "list staged objects after recover")
	if len(staged) != 1 || staged[0].Key != "fresh.txt" {
		t.Fatalf("staged objects after recover = %+v, want only fresh pending object", staged)
	}
	assertRestartShardExists(ctx, t, eng, scenario.freshBlob, "fresh pending shard was removed during recovery")
	assertRestartShardMissing(ctx, t, eng, scenario.replacementBlob, "expired replacement shard still exists after rollback cleanup")
	assertRestartShardMissing(ctx, t, eng, scenario.retainedBlob, "expired retained shard still exists after release")
	if _, exists, getErr := scenario.meta.GetBlobRef(ctx, scenario.retainedBlob.Hash); getErr != nil || exists {
		t.Fatalf("retained blob ref exists = %v err = %v, want removed", exists, getErr)
	}
}

func assertRestartRecoveryCounts(t *testing.T, got, want map[string]int, label string) {
	t.Helper()

	for key, wantValue := range want {
		if got[key] != wantValue {
			t.Fatalf("%s = %+v, want %s=%d", label, got, key, wantValue)
		}
	}
}

func assertRestartShardExists(
	ctx context.Context,
	t *testing.T,
	eng *engine.Engine,
	blob engine.BlobInfo,
	message string,
) {
	t.Helper()

	if !eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal(message)
	}
}

func assertRestartShardMissing(
	ctx context.Context,
	t *testing.T,
	eng *engine.Engine,
	blob engine.BlobInfo,
	message string,
) {
	t.Helper()

	if eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal(message)
	}
}

func restartRecoveryActionCounts(actions []store.PendingRecoveryAction) map[string]int {
	counts := make(map[string]int)
	for index := range actions {
		counts[actions[index].Action]++
	}
	return counts
}
