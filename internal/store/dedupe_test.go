package store_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
)

func TestStoreDedupeRepairsBlobRefCountDrift(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	storeModule, err := store.NewStore(t.TempDir(), meta, nil)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")
	first, err := storeModule.PutObject(ctx, "bucket", "first.txt", bytes.NewReader([]byte("same payload")), store.PutOptions{})
	mustNoError(t, err, "put first")
	_, err = storeModule.PutObject(ctx, "bucket", "second.txt", bytes.NewReader([]byte("same payload")), store.PutOptions{})
	mustNoError(t, err, "put second")
	_, _, err = meta.DecreaseBlobRef(ctx, first.Hash)
	mustNoError(t, err, "decrease blob ref")

	plan, err := storeModule.PlanDedupe(ctx)
	mustNoError(t, err, "plan dedupe")
	if plan.RefCountDrift != 1 || plan.RefCountIncreased != 0 {
		t.Fatalf("plan = %+v, want one dry-run drift without mutation", plan)
	}
	result, err := storeModule.Dedupe(ctx, store.DedupeOptions{})
	mustNoError(t, err, "run dedupe")
	if result.RefCountIncreased != 1 {
		t.Fatalf("ref count increased = %d, want 1", result.RefCountIncreased)
	}
	ref, _, err := meta.GetBlobRef(ctx, first.Hash)
	mustNoError(t, err, "get ref")
	if ref.RefCount != 2 {
		t.Fatalf("ref count = %d, want 2", ref.RefCount)
	}
}

func TestStoreDedupeRemovesOrphanBlobRef(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	blob, err := eng.PutBlob(ctx, "orphan.txt", bytes.NewReader([]byte("orphan payload")))
	mustNoError(t, err, "put blob")
	mustNoError(t, meta.CreateBlobRef(ctx, blob.Hash, blob.ShardDir, blob.Size, blob.ShardPlacements, blob.ShardChecksums), "create blob ref")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")

	result, err := storeModule.Dedupe(ctx, store.DedupeOptions{})
	mustNoError(t, err, "run dedupe")
	if result.OrphanBlobRefsRemoved != 1 {
		t.Fatalf("orphan blob refs removed = %d, want 1", result.OrphanBlobRefsRemoved)
	}
	if _, ok, err := meta.GetBlobRef(ctx, blob.Hash); err != nil || ok {
		t.Fatalf("blob ref exists = %v err = %v, want removed", ok, err)
	}
	if eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal("orphan blob shard still exists")
	}
}
