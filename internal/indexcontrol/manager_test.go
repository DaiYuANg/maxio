package indexcontrol

import (
	"context"
	"errors"
	"testing"
	"time"

	searchindex "github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestManagerStatusUnavailable(t *testing.T) {
	ctx := context.Background()

	var nilManager *Manager
	requireErrorIs(t, errFromStatus(ctx, nilManager), ErrManagerUnavailable, "nil manager status")

	manager := &Manager{search: searchindex.NewInMemorySearchEngine()}
	requireErrorIs(t, errFromStatus(ctx, manager), ErrManagerUnavailable, "missing metadata status")
}

func TestManagerRebuildUnavailable(t *testing.T) {
	ctx := context.Background()

	var nilManager *Manager
	requireErrorIs(t, errFromRebuild(ctx, nilManager), ErrManagerUnavailable, "nil manager rebuild")

	managerWithoutSearch := NewManager(metadata.NewInMemoryMetadata(), nil, ManagerOptions{})
	requireErrorIs(t, errFromRebuild(ctx, managerWithoutSearch), ErrManagerUnavailable, "missing search rebuild")

	managerWithoutMetadata := &Manager{search: searchindex.NewInMemorySearchEngine()}
	requireErrorIs(t, errFromRebuild(ctx, managerWithoutMetadata), ErrManagerUnavailable, "missing metadata rebuild")
}

func TestManagerRebuildInProgress(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(metadata.NewInMemoryMetadata(), searchindex.NewInMemorySearchEngine(), ManagerOptions{})

	if !manager.beginRebuild(time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("expected rebuild start")
	}

	requireErrorIs(t, errFromRebuild(ctx, manager), ErrRebuildInProgress, "rebuild in progress")
}

func TestManagerStatusFromMetadataSnapshot(t *testing.T) {
	ctx := context.Background()
	store := metadata.NewInMemoryMetadata()
	manager := NewManager(store, searchindex.NewInMemorySearchEngine(), ManagerOptions{})

	seedStatusSnapshotRows(ctx, t, store)
	status := requireStatus(ctx, t, manager)

	assertBool(t, "rebuilding", status.Rebuilding, false)
	assertInt(t, "queue_size", status.QueueSize, 1)
	assertInt(t, "queued_objects", status.QueuedObjects, 2)
	assertInt(t, "retried_objects", status.RetriedObjects, 1)
	assertInt(t, "indexed_objects", status.IndexedObjects, 1)
	assertInt(t, "failed_objects", status.FailedObjects, 2)
}

func TestManagerStatusReflectsRebuildLifecycle(t *testing.T) {
	ctx := context.Background()
	store := metadata.NewInMemoryMetadata()
	manager := NewManager(store, searchindex.NewInMemorySearchEngine(), ManagerOptions{})
	startedAt := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)

	if !manager.beginRebuild(startedAt) {
		t.Fatal("expected initial begin")
	}
	assertActiveRebuildStatus(t, requireStatus(ctx, t, manager), startedAt)

	finishedAt := startedAt.Add(2 * time.Minute)
	manager.finishRebuild(RebuildResult{StartedAt: startedAt, FinishedAt: finishedAt, Objects: 2, Failed: 1}, "temporary indexer error")
	assertFinishedRebuildStatus(t, requireStatus(ctx, t, manager), startedAt, finishedAt)
}

func TestManagerRebuildRebuildsCommittedObjectsAndPersistsJobState(t *testing.T) {
	ctx := context.Background()
	store := metadata.NewInMemoryMetadata()
	search := searchindex.NewInMemorySearchEngine()
	buildStartedAt := time.Date(2026, 7, 18, 8, 0, 0, 0, time.UTC)
	manager := NewManager(store, search, ManagerOptions{Now: tickingClock(buildStartedAt, time.Second)})

	seedCommittedObject(ctx, t, store)
	result := requireRebuild(ctx, t, manager)

	assertRebuildResult(t, result, buildStartedAt)
	assertIndexJob(ctx, t, store, rebuildJobID(result.StartedAt), model.IndexJobStatusSucceeded)
	assertIndexDocument(ctx, t, store, "bucket-a\x00alpha\x00v1", model.IndexDocumentStateIndexed)
}

func errFromStatus(ctx context.Context, manager *Manager) error {
	_, err := manager.Status(ctx)
	return err
}

func errFromRebuild(ctx context.Context, manager *Manager) error {
	_, err := manager.Rebuild(ctx)
	return err
}

func requireErrorIs(t *testing.T, err, target error, action string) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("%s error = %v, want %v", action, err, target)
	}
}

func requireNoError(t *testing.T, err error, action string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", action, err)
	}
}

func requireStatus(ctx context.Context, t *testing.T, manager *Manager) Status {
	t.Helper()
	status, err := manager.Status(ctx)
	requireNoError(t, err, "status")
	return status
}

func requireRebuild(ctx context.Context, t *testing.T, manager *Manager) RebuildResult {
	t.Helper()
	result, err := manager.Rebuild(ctx)
	requireNoError(t, err, "rebuild")
	return result
}

func seedStatusSnapshotRows(ctx context.Context, t *testing.T, store *metadata.InMemoryMetadata) {
	t.Helper()
	seedIndexDocuments(ctx, t, store, []model.IndexDocument{
		{Bucket: "bucket-a", Key: "alpha", VersionID: "v1", State: model.IndexDocumentStateIndexed},
		{Bucket: "bucket-a", Key: "bravo", VersionID: "v2", State: model.IndexDocumentStatePending},
		{Bucket: "bucket-a", Key: "charlie", VersionID: "v3", State: model.IndexDocumentStateFailed, Error: "failed index document"},
	})
	seedIndexJobs(ctx, t, store, []model.IndexJob{
		{ID: "job-queued", Kind: model.IndexJobKindUpsert, Status: model.IndexJobStatusQueued},
		{ID: "job-retried-failed", Kind: model.IndexJobKindDelete, Status: model.IndexJobStatusFailed, Attempts: 2, Error: "failed index job"},
	})
}

func seedIndexDocuments(ctx context.Context, t *testing.T, store *metadata.InMemoryMetadata, docs []model.IndexDocument) {
	t.Helper()
	for i := range docs {
		_, err := store.UpsertIndexDocument(ctx, docs[i])
		requireNoError(t, err, "upsert index document")
	}
}

func seedIndexJobs(ctx context.Context, t *testing.T, store *metadata.InMemoryMetadata, jobs []model.IndexJob) {
	t.Helper()
	for i := range jobs {
		_, err := store.UpsertIndexJob(ctx, jobs[i])
		requireNoError(t, err, "upsert index job")
	}
}

func seedCommittedObject(ctx context.Context, t *testing.T, store *metadata.InMemoryMetadata) {
	t.Helper()
	requireNoError(t, store.CreateBucket(ctx, "bucket-a"), "create bucket")
	requireNoError(t, store.UpsertObjectMeta(ctx, model.ObjectMeta{Bucket: "bucket-a", Key: "alpha", Hash: "sha256:abcdef"}), "upsert object meta")
	_, err := store.UpsertObjectRecord(ctx, model.ObjectRecord{Bucket: "bucket-a", Key: "alpha", CurrentVersionID: "v1"})
	requireNoError(t, err, "upsert object record")
}

func tickingClock(start time.Time, step time.Duration) func() time.Time {
	next := start
	return func() time.Time {
		current := next
		next = next.Add(step)
		return current
	}
}

func assertActiveRebuildStatus(t *testing.T, status Status, startedAt time.Time) {
	t.Helper()
	assertBool(t, "rebuilding", status.Rebuilding, true)
	assertTime(t, "last_rebuild_started_at", status.LastRebuildStartedAt, startedAt)
	assertZeroTime(t, "last_rebuild_finished_at", status.LastRebuildFinishedAt)
}

func assertFinishedRebuildStatus(t *testing.T, status Status, startedAt, finishedAt time.Time) {
	t.Helper()
	assertBool(t, "rebuilding", status.Rebuilding, false)
	assertTime(t, "last_rebuild_started_at", status.LastRebuildStartedAt, startedAt)
	assertTime(t, "last_rebuild_finished_at", status.LastRebuildFinishedAt, finishedAt)
	assertInt(t, "last_rebuild_objects", status.LastRebuildObjects, 2)
	assertInt(t, "last_rebuild_failed", status.LastRebuildFailed, 1)
	assertString(t, "last_rebuild_error", status.LastRebuildError, "temporary indexer error")
}

func assertRebuildResult(t *testing.T, result RebuildResult, startedAt time.Time) {
	t.Helper()
	assertInt(t, "rebuild objects", result.Objects, 1)
	assertInt(t, "rebuild failed", result.Failed, 0)
	assertTime(t, "result.started_at", result.StartedAt, startedAt)
	if !result.FinishedAt.After(result.StartedAt) {
		t.Fatal("expected distinct started and finished times")
	}
}

func assertIndexJob(ctx context.Context, t *testing.T, store *metadata.InMemoryMetadata, id, status string) {
	t.Helper()
	job, found, err := store.GetIndexJob(ctx, id)
	requireNoError(t, err, "get index job")
	if !found {
		t.Fatalf("expected index job %q", id)
	}
	assertString(t, "rebuild job status", job.Status, status)
	assertString(t, "rebuild job error", job.Error, "")
}

func assertIndexDocument(ctx context.Context, t *testing.T, store *metadata.InMemoryMetadata, id, state string) {
	t.Helper()
	document, found, err := store.GetIndexDocument(ctx, id)
	requireNoError(t, err, "get index document")
	if !found {
		t.Fatalf("expected index document %q", id)
	}
	assertString(t, "index document state", document.State, state)
}

func assertBool(t *testing.T, name string, got, want bool) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func assertInt(t *testing.T, name string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %d, want %d", name, got, want)
	}
}

func assertString(t *testing.T, name, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s = %q, want %q", name, got, want)
	}
}

func assertTime(t *testing.T, name string, got, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Fatalf("%s = %s, want %s", name, got, want)
	}
}

func assertZeroTime(t *testing.T, name string, got time.Time) {
	t.Helper()
	if !got.IsZero() {
		t.Fatalf("%s = %s, want zero", name, got)
	}
}
