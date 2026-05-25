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

func newRestartRecoveryScenario(ctx context.Context, t *testing.T) restartRecoveryScenario {
	t.Helper()

	dataDir := t.TempDir()
	meta := metadata.NewInMemoryMetadata()
	initialEngine, initialStore := newRestartRecoveryEngineAndStore(ctx, t, dataDir, meta)
	now := time.Now().UTC()
	expiredAt := now.Add(-2 * time.Hour)
	original := []byte("original committed payload after restart")
	committed := putRestartCommittedObject(ctx, t, initialStore, original)
	freshBlob := stageRestartFreshPending(ctx, t, meta, initialEngine, now)
	stageRestartMetadataOnlyPending(ctx, t, meta, expiredAt)
	replacementBlob := stageRestartLayoutLinkedPending(ctx, t, meta, initialEngine, expiredAt)
	retainedBlob := stageRestartRetainedPending(ctx, t, meta, initialEngine, expiredAt)

	return restartRecoveryScenario{
		dataDir:         dataDir,
		meta:            meta,
		original:        original,
		committed:       committed,
		freshBlob:       freshBlob,
		replacementBlob: replacementBlob,
		retainedBlob:    retainedBlob,
	}
}

func newRestartRecoveryEngineAndStore(
	ctx context.Context,
	t *testing.T,
	dataDir string,
	meta *metadata.InMemoryMetadata,
) (*engine.Engine, *store.Store) {
	t.Helper()

	eng, err := engine.NewEngine(dataDir, engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new initial engine")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new initial store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")
	return eng, storeModule
}

func newRestartedRecoveryStore(
	t *testing.T,
	dataDir string,
	meta *metadata.InMemoryMetadata,
) (*store.Store, *engine.Engine) {
	t.Helper()

	eng, err := engine.NewEngine(dataDir, engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	mustNoError(t, err, "new restarted engine")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new restarted store")
	return storeModule, eng
}

func putRestartCommittedObject(
	ctx context.Context,
	t *testing.T,
	storeModule *store.Store,
	original []byte,
) model.ObjectMeta {
	t.Helper()

	committed, err := storeModule.PutObject(ctx, "bucket", "object.txt", bytes.NewReader(original), store.PutOptions{
		ContentType: "text/plain",
	})
	mustNoError(t, err, "put committed object")
	return committed
}

func stageRestartFreshPending(
	ctx context.Context,
	t *testing.T,
	meta *metadata.InMemoryMetadata,
	eng *engine.Engine,
	at time.Time,
) engine.BlobInfo {
	t.Helper()

	blob, err := eng.PutBlob(ctx, "fresh.txt", bytes.NewReader([]byte("fresh pending payload")))
	mustNoError(t, err, "put fresh pending blob")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "fresh.txt",
		Hash:      blob.Hash,
		ETag:      engine.ETagFromHash(blob.Hash),
		Size:      blob.Size,
		State:     model.ObjectStatePending,
		UpdatedAt: at,
		WriteIntent: restartRecoveryIntent(
			model.WriteIntentStageBlobPrepared,
			at,
		),
	}), "stage fresh pending object")
	return blob
}

func stageRestartMetadataOnlyPending(ctx context.Context, t *testing.T, meta *metadata.InMemoryMetadata, at time.Time) {
	t.Helper()

	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "metadata-only.txt",
		Hash:      "metadata-only-hash",
		State:     model.ObjectStatePending,
		UpdatedAt: at,
		WriteIntent: restartRecoveryIntent(
			model.WriteIntentStageMetadataStaged,
			at,
		),
	}), "stage expired metadata-only pending object")
}

func stageRestartLayoutLinkedPending(
	ctx context.Context,
	t *testing.T,
	meta *metadata.InMemoryMetadata,
	eng *engine.Engine,
	at time.Time,
) engine.BlobInfo {
	t.Helper()

	blob, err := eng.PutBlob(ctx, "object.txt", bytes.NewReader([]byte("replacement payload")))
	mustNoError(t, err, "put replacement blob")
	_, err = eng.LinkObject(ctx, "bucket", "object.txt", blob, "text/plain", at)
	mustNoError(t, err, "link replacement layout")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "object.txt",
		Hash:      blob.Hash,
		ETag:      engine.ETagFromHash(blob.Hash),
		Size:      blob.Size,
		State:     model.ObjectStatePending,
		UpdatedAt: at,
		WriteIntent: restartRecoveryIntent(
			model.WriteIntentStageLayoutLinked,
			at,
		),
	}), "stage expired layout-linked pending object")
	return blob
}

func stageRestartRetainedPending(
	ctx context.Context,
	t *testing.T,
	meta *metadata.InMemoryMetadata,
	eng *engine.Engine,
	at time.Time,
) engine.BlobInfo {
	t.Helper()

	blob, err := eng.PutBlob(ctx, "retained.txt", bytes.NewReader([]byte("retained pending payload")))
	mustNoError(t, err, "put retained pending blob")
	mustNoError(t, meta.CreateBlobRef(
		ctx,
		blob.Hash,
		blob.ShardDir,
		blob.Size,
		blob.ShardPlacements,
		blob.ShardChecksums,
		blob.ShardSizes,
	), "create retained blob ref")
	mustNoError(t, meta.StageObjectMeta(ctx, model.ObjectMeta{
		Bucket:    "bucket",
		Key:       "retained.txt",
		Hash:      blob.Hash,
		ETag:      engine.ETagFromHash(blob.Hash),
		Size:      blob.Size,
		State:     model.ObjectStatePending,
		UpdatedAt: at,
		WriteIntent: restartRecoveryIntent(
			model.WriteIntentStageBlobRetained,
			at,
		),
	}), "stage expired retained pending object")
	return blob
}

func restartRecoveryIntent(stage string, at time.Time) *model.WriteIntent {
	return &model.WriteIntent{
		Stage:     stage,
		StartedAt: at,
		UpdatedAt: at,
	}
}
