package store_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
)

func TestStorePutObjectDedupePreservesBlobRefPlacements(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	storeModule, err := store.NewStore(t.TempDir(), meta, nil)
	mustNoError(t, err, "new store")

	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")

	first, err := storeModule.PutObject(ctx, "bucket", "first.txt", strings.NewReader("hello dedupe"), store.PutOptions{
		ContentType: "text/plain",
	})
	mustNoError(t, err, "put first object")
	second, err := storeModule.PutObject(ctx, "bucket", "second.txt", strings.NewReader("hello dedupe"), store.PutOptions{
		ContentType: "text/plain",
	})
	mustNoError(t, err, "put second object")

	mustEqual(t, first.Hash, second.Hash, "hashes must match")
	mustEqual(t, first.WriteIntent.Stage, model.WriteIntentStageCommitted, "first write intent stage")
	mustEqual(t, second.WriteIntent.Stage, model.WriteIntentStageCommitted, "second write intent stage")
	mustDeepEqual(t, first.ShardPlacements, second.ShardPlacements, "shard placements should match across objects")
	mustDeepEqual(t, first.ShardChecksums, second.ShardChecksums, "shard checksums should match across objects")

	ref, _, err := meta.GetBlobRef(ctx, first.Hash)
	mustNoError(t, err, "get blob ref")
	mustEqual(t, ref.RefCount, 2, "blob ref count should be increased")
	mustDeepEqual(t, ref.ShardPlacements, first.ShardPlacements, "blob ref should keep shard placements")
	mustDeepEqual(t, ref.ShardChecksums, first.ShardChecksums, "blob ref should keep shard checksums")
}

func mustNoError(t *testing.T, err error, format string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", format, err)
	}
}

func mustEqual[T comparable](t *testing.T, got, want T, format string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: got %v, want %v", format, got, want)
	}
}

func mustDeepEqual[T any](t *testing.T, got, want T, format string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: %#v != %#v", format, got, want)
	}
}
