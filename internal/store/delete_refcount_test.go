package store_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
)

func TestStoreDeleteKeepsSharedDedupeBlobUntilLastReference(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")

	first := putSharedObject(ctx, t, storeModule, "first.txt")
	second := putSharedObject(ctx, t, storeModule, "second.txt")
	if first.Hash != second.Hash {
		t.Fatalf("hashes = %s/%s, want shared blob", first.Hash, second.Hash)
	}

	ref, exists, err := meta.GetBlobRef(ctx, first.Hash)
	mustNoError(t, err, "get shared blob ref")
	if !exists {
		t.Fatal("shared blob ref does not exist")
	}
	if ref.RefCount != 2 {
		t.Fatalf("ref count = %d, want 2", ref.RefCount)
	}

	_, err = storeModule.DeleteObject(ctx, "bucket", "first.txt")
	mustNoError(t, err, "delete first object")
	assertSharedBlobStillRetained(ctx, t, meta, eng, ref)
	assertSharedObjectReadable(ctx, t, storeModule)

	_, err = storeModule.DeleteObject(ctx, "bucket", "second.txt")
	mustNoError(t, err, "delete second object")
	assertSharedBlobReleased(ctx, t, meta, eng, ref)
}

func putSharedObject(ctx context.Context, t *testing.T, storeModule *store.Store, key string) model.ObjectMeta {
	t.Helper()
	meta, err := storeModule.PutObject(
		ctx,
		"bucket",
		key,
		bytes.NewReader([]byte("shared dedupe payload")),
		store.PutOptions{ContentType: "text/plain"},
	)
	mustNoError(t, err, "put shared object")
	return meta
}

func assertSharedBlobStillRetained(
	ctx context.Context,
	t *testing.T,
	meta *metadata.InMemoryMetadata,
	eng *engine.Engine,
	ref metadata.BlobRef,
) {
	t.Helper()
	updated, exists, err := meta.GetBlobRef(ctx, ref.Hash)
	mustNoError(t, err, "get updated shared blob ref")
	if !exists {
		t.Fatal("shared blob ref was removed while still referenced")
	}
	if updated.RefCount != 1 {
		t.Fatalf("ref count = %d, want 1", updated.RefCount)
	}
	if !eng.LocalShardExists(ctx, ref.Path, ref.Hash, 0) {
		t.Fatal("shared blob shard was removed while still referenced")
	}
}

func assertSharedObjectReadable(ctx context.Context, t *testing.T, storeModule *store.Store) {
	t.Helper()
	reader, _, err := storeModule.GetObject(ctx, "bucket", "second.txt")
	mustNoError(t, err, "get remaining shared object")
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close reader: %v", closeErr)
		}
	}()
	data, err := io.ReadAll(reader)
	mustNoError(t, err, "read remaining shared object")
	if !bytes.Equal(data, []byte("shared dedupe payload")) {
		t.Fatalf("data = %q, want shared dedupe payload", data)
	}
}

func assertSharedBlobReleased(
	ctx context.Context,
	t *testing.T,
	meta *metadata.InMemoryMetadata,
	eng *engine.Engine,
	ref metadata.BlobRef,
) {
	t.Helper()
	_, exists, err := meta.GetBlobRef(ctx, ref.Hash)
	mustNoError(t, err, "get released shared blob ref")
	if exists {
		t.Fatal("shared blob ref still exists after last delete")
	}
	if eng.LocalShardExists(ctx, ref.Path, ref.Hash, 0) {
		t.Fatal("shared blob shard still exists after last delete")
	}
}
