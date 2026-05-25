package store_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
)

func TestStoreRecoverRemovesExpiredPendingObjects(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	storeModule, err := store.NewStore(t.TempDir(), meta, nil)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "pending.txt",
		Hash:      "pending-hash",
		UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}), "stage pending object")

	result, err := storeModule.Recover(ctx, store.RecoveryOptions{PendingTTL: time.Hour})
	mustNoError(t, err, "recover store")
	if result.PendingRemoved != 1 {
		t.Fatalf("pending removed = %d, want 1", result.PendingRemoved)
	}
	if result.PendingActions[store.PendingRecoveryActionDeleteStaged] != 1 {
		t.Fatalf("pending actions = %+v, want one delete staged", result.PendingActions)
	}
	staged, err := meta.ListStagedObjectMetas(ctx, "", "")
	mustNoError(t, err, "list staged objects")
	if len(staged) != 0 {
		t.Fatalf("staged objects = %+v, want empty", staged)
	}
}

func TestStoreRecoverRemovesOrphanShardSets(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	blob, err := eng.PutBlob(ctx, "orphan.txt", bytes.NewReader([]byte("orphan payload")))
	mustNoError(t, err, "put orphan blob")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")

	result, err := storeModule.Recover(ctx, store.RecoveryOptions{
		PendingTTL:          time.Hour,
		CleanupOrphanShards: true,
	})
	mustNoError(t, err, "recover store")
	if result.OrphanShardCleanup.Removed != 1 {
		t.Fatalf("orphan removed = %d, want 1", result.OrphanShardCleanup.Removed)
	}
	if eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal("orphan shard still exists")
	}
}

func TestStoreRecoveryPlanReportsPendingAndOrphansWithoutMutating(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	blob, err := eng.PutBlob(ctx, "planned-orphan.txt", bytes.NewReader([]byte("planned orphan")))
	mustNoError(t, err, "put planned orphan blob")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "expired.txt",
		Hash:      "expired-hash",
		UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}), "stage expired pending object")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "fresh.txt",
		Hash:      "fresh-hash",
		UpdatedAt: time.Now().UTC(),
	}), "stage fresh pending object")

	plan, err := storeModule.PlanRecovery(ctx, time.Hour)
	mustNoError(t, err, "plan recovery")
	if len(plan.PendingObjects) != 2 {
		t.Fatalf("pending objects = %d, want 2", len(plan.PendingObjects))
	}
	if len(plan.ExpiredPendingObjects) != 1 || plan.ExpiredPendingObjects[0].Key != "expired.txt" {
		t.Fatalf("expired pending objects = %+v", plan.ExpiredPendingObjects)
	}
	if plan.WriteIntentStages[model.WriteIntentStageUnknown] != 2 {
		t.Fatalf("write intent stages = %+v, want two unknown pending objects", plan.WriteIntentStages)
	}
	if len(plan.PendingActions) != 2 {
		t.Fatalf("pending actions = %+v, want two actions", plan.PendingActions)
	}
	if plan.OrphanShardCleanup.Removed != 0 || len(plan.OrphanShardCleanup.Orphans) != 1 {
		t.Fatalf("orphan cleanup plan = %+v", plan.OrphanShardCleanup)
	}
	staged, err := meta.ListStagedObjectMetas(ctx, "", "")
	mustNoError(t, err, "list staged after recovery plan")
	if len(staged) != 2 {
		t.Fatalf("staged objects after plan = %d, want 2", len(staged))
	}
	if !eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal("recovery plan removed orphan shard")
	}
}

func TestStoreRecoverRollsBackBlobRetainedPendingWrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	blob, err := eng.PutBlob(ctx, "pending.txt", bytes.NewReader([]byte("pending payload")))
	mustNoError(t, err, "put pending blob")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")
	mustNoError(t, meta.CreateBlobRef(ctx, blob.Hash, blob.ShardDir, blob.Size, blob.ShardPlacements, blob.ShardChecksums), "create blob ref")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket: "bucket",
		Key:    "pending.txt",
		Hash:   blob.Hash,
		WriteIntent: &model.WriteIntent{
			Stage:     model.WriteIntentStageBlobRetained,
			StartedAt: time.Now().UTC().Add(-2 * time.Hour),
			UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
		},
		UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
	}), "stage blob retained pending object")

	result, err := storeModule.Recover(ctx, store.RecoveryOptions{
		PendingTTL:          time.Hour,
		CleanupOrphanShards: true,
	})
	mustNoError(t, err, "recover store")
	if result.PendingRemoved != 1 {
		t.Fatalf("pending removed = %d, want 1", result.PendingRemoved)
	}
	if result.PendingActions[store.PendingRecoveryActionReleaseBlob] != 1 {
		t.Fatalf("pending actions = %+v, want one release blob", result.PendingActions)
	}
	if _, ok, err := meta.GetBlobRef(ctx, blob.Hash); err != nil || ok {
		t.Fatalf("blob ref exists = %v err = %v, want removed", ok, err)
	}
	if eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal("pending blob shard still exists")
	}
}

func TestStoreRecoverDoesNotDeleteFreshPendingBlobPreparedShards(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	blob, err := eng.PutBlob(ctx, "pending.txt", bytes.NewReader([]byte("fresh pending payload")))
	mustNoError(t, err, "put pending blob")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "pending.txt",
		Hash:      blob.Hash,
		UpdatedAt: time.Now().UTC(),
		WriteIntent: &model.WriteIntent{
			Stage:     model.WriteIntentStageBlobPrepared,
			StartedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		},
	}), "stage fresh blob prepared pending object")

	result, err := storeModule.Recover(ctx, store.RecoveryOptions{
		PendingTTL:          time.Hour,
		CleanupOrphanShards: true,
	})
	mustNoError(t, err, "recover store")
	if result.PendingRemoved != 0 {
		t.Fatalf("pending removed = %d, want 0", result.PendingRemoved)
	}
	if len(result.PendingActions) != 0 {
		t.Fatalf("pending actions = %+v, want empty", result.PendingActions)
	}
	if result.OrphanShardCleanup.Removed != 0 {
		t.Fatalf("orphan shards removed = %d, want 0", result.OrphanShardCleanup.Removed)
	}
	if !eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal("fresh pending blob shard was removed")
	}
}
