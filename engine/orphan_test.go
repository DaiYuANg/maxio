package engine_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
)

func TestCleanupOrphanShardSetsRemovesUnreferencedLocalBlob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	blob, err := eng.PutBlob(ctx, "orphan.txt", bytes.NewReader([]byte("orphan payload")))
	if err != nil {
		t.Fatalf("put orphan blob: %v", err)
	}

	result, err := eng.CleanupOrphanShardSets(ctx, nil, false)
	if err != nil {
		t.Fatalf("cleanup orphan shard sets: %v", err)
	}
	if result.Scanned != 1 || result.Removed != 1 {
		t.Fatalf("cleanup result = %+v, want scanned=1 removed=1", result)
	}
	if eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal("orphan shard still exists")
	}
}

func TestCleanupOrphanShardSetsKeepsReferencedBlob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	blob, err := eng.PutBlob(ctx, "live.txt", bytes.NewReader([]byte("live payload")))
	if err != nil {
		t.Fatalf("put live blob: %v", err)
	}

	result, err := eng.CleanupOrphanShardSets(ctx, []engine.ShardSetRef{
		{ShardDir: blob.ShardDir, Hash: blob.Hash},
	}, false)
	if err != nil {
		t.Fatalf("cleanup live shard sets: %v", err)
	}
	if result.Scanned != 1 || result.Removed != 0 {
		t.Fatalf("cleanup result = %+v, want scanned=1 removed=0", result)
	}
	if !eng.LocalShardExists(ctx, blob.ShardDir, blob.Hash, 0) {
		t.Fatal("live shard was removed")
	}
}
