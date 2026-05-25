package store_test

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
)

func TestStoreRecoverRestoresCommittedLayoutAfterExpiredOverwrite(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")

	original := []byte("original committed payload")
	committed, err := storeModule.PutObject(ctx, "bucket", "object.txt", bytes.NewReader(original), store.PutOptions{
		ContentType: "text/plain",
	})
	mustNoError(t, err, "put committed object")

	replacement, err := eng.PutBlob(ctx, "object.txt", bytes.NewReader([]byte("replacement payload")))
	mustNoError(t, err, "put replacement blob")
	_, err = eng.LinkObject(ctx, "bucket", "object.txt", replacement, "text/plain", time.Now().UTC())
	mustNoError(t, err, "link replacement layout")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "object.txt",
		Hash:      replacement.Hash,
		ETag:      engine.ETagFromHash(replacement.Hash),
		Size:      replacement.Size,
		State:     model.ObjectStatePending,
		UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
		WriteIntent: &model.WriteIntent{
			Stage:     model.WriteIntentStageLayoutLinked,
			StartedAt: time.Now().UTC().Add(-2 * time.Hour),
			UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
		},
	}), "stage expired replacement object")

	result, err := storeModule.Recover(ctx, store.RecoveryOptions{
		PendingTTL:          time.Hour,
		CleanupOrphanShards: true,
	})
	mustNoError(t, err, "recover store")
	if result.PendingRemoved != 1 {
		t.Fatalf("pending removed = %d, want 1", result.PendingRemoved)
	}
	if result.PendingActions[store.PendingRecoveryActionRollbackLayout] != 1 {
		t.Fatalf("pending actions = %+v, want one rollback layout", result.PendingActions)
	}
	if eng.LocalShardExists(ctx, replacement.ShardDir, replacement.Hash, 0) {
		t.Fatal("replacement shard still exists")
	}

	assertRecoveredObject(ctx, t, storeModule, original, committed.Hash)
}

func TestStoreRecoverReleasesRetainedOverwriteBlobAndKeepsCommittedObject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")

	original := []byte("original retained committed payload")
	committed, err := storeModule.PutObject(ctx, "bucket", "object.txt", bytes.NewReader(original), store.PutOptions{
		ContentType: "text/plain",
	})
	mustNoError(t, err, "put committed object")

	replacement, err := eng.PutBlob(ctx, "object.txt", bytes.NewReader([]byte("retained replacement payload")))
	mustNoError(t, err, "put replacement blob")
	_, err = eng.LinkObject(ctx, "bucket", "object.txt", replacement, "text/plain", time.Now().UTC())
	mustNoError(t, err, "link replacement layout")
	mustNoError(t, meta.CreateBlobRef(
		ctx,
		replacement.Hash,
		replacement.ShardDir,
		replacement.Size,
		replacement.ShardPlacements,
		replacement.ShardChecksums,
		replacement.ShardSizes,
	), "create replacement blob ref")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "object.txt",
		Hash:      replacement.Hash,
		ETag:      engine.ETagFromHash(replacement.Hash),
		Size:      replacement.Size,
		State:     model.ObjectStatePending,
		UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
		WriteIntent: &model.WriteIntent{
			Stage:     model.WriteIntentStageBlobRetained,
			StartedAt: time.Now().UTC().Add(-2 * time.Hour),
			UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
		},
	}), "stage expired retained replacement object")

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
	if _, exists, getErr := meta.GetBlobRef(ctx, replacement.Hash); getErr != nil || exists {
		t.Fatalf("replacement blob ref exists = %v err = %v, want removed", exists, getErr)
	}
	if eng.LocalShardExists(ctx, replacement.ShardDir, replacement.Hash, 0) {
		t.Fatal("replacement shard still exists")
	}

	assertRecoveredObject(ctx, t, storeModule, original, committed.Hash)
}

func assertRecoveredObject(
	ctx context.Context,
	t *testing.T,
	storeModule *store.Store,
	expected []byte,
	expectedHash string,
) {
	t.Helper()

	reader, recovered, err := storeModule.GetObject(ctx, "bucket", "object.txt")
	mustNoError(t, err, "get recovered object")
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close reader: %v", closeErr)
		}
	}()
	data, err := io.ReadAll(reader)
	mustNoError(t, err, "read recovered object")
	if !bytes.Equal(data, expected) {
		t.Fatalf("recovered data = %q, want %q", data, expected)
	}
	if recovered.Hash != expectedHash {
		t.Fatalf("recovered hash = %s, want %s", recovered.Hash, expectedHash)
	}
}
