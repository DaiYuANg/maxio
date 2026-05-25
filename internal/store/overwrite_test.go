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

func TestStoreOverwriteReleasesReplacedBlob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new engine")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")

	first := putObjectForOverwrite(ctx, t, storeModule, "first payload")
	firstRef, exists, err := meta.GetBlobRef(ctx, first.Hash)
	mustNoError(t, err, "get first blob ref")
	if !exists {
		t.Fatal("first blob ref does not exist")
	}

	second := putObjectForOverwrite(ctx, t, storeModule, "second payload")

	if first.Hash == second.Hash {
		t.Fatal("overwrite test requires different object hashes")
	}
	if _, exists, err := meta.GetBlobRef(ctx, first.Hash); err != nil || exists {
		t.Fatalf("first blob ref exists = %v err = %v, want removed", exists, err)
	}
	if eng.LocalShardExists(ctx, firstRef.Path, first.Hash, 0) {
		t.Fatal("first blob shard still exists after overwrite")
	}
	assertStoredObjectPayload(ctx, t, storeModule, []byte("second payload"), second.Hash)
}

func putObjectForOverwrite(
	ctx context.Context,
	t *testing.T,
	storeModule *store.Store,
	payload string,
) model.ObjectMeta {
	t.Helper()
	meta, err := storeModule.PutObject(ctx, "bucket", "object.txt", bytes.NewReader([]byte(payload)), store.PutOptions{
		ContentType: "text/plain",
	})
	mustNoError(t, err, "put overwrite object")
	return meta
}

func assertStoredObjectPayload(
	ctx context.Context,
	t *testing.T,
	storeModule *store.Store,
	expected []byte,
	expectedHash string,
) {
	t.Helper()
	reader, meta, err := storeModule.GetObject(ctx, "bucket", "object.txt")
	mustNoError(t, err, "get overwritten object")
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close reader: %v", closeErr)
		}
	}()
	data, err := io.ReadAll(reader)
	mustNoError(t, err, "read overwritten object")
	if !bytes.Equal(data, expected) {
		t.Fatalf("data = %q, want %q", data, expected)
	}
	if meta.Hash != expectedHash {
		t.Fatalf("hash = %s, want %s", meta.Hash, expectedHash)
	}
}
