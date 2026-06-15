package metadata

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestSQLMetadataCatalogLifecycle(t *testing.T) {
	ctx := context.Background()
	store := newTestSQLiteCatalogStore(t)
	createTestBucket(ctx, t, store, "photos")

	digest := retainTestDigestRef(ctx, t, store)
	version := upsertTestObjectVersion(ctx, t, store, digest)
	upsertTestObjectRecord(ctx, t, store, version)
	assertTestObjectVersions(ctx, t, store)
	upsertTestIndexDocument(ctx, t, store)
	upsertTestIndexJob(ctx, t, store)
	upsertTestIndexOutboxEvent(ctx, t, store)
	deleteTestCatalogEntries(ctx, t, store)
}

func newTestSQLiteCatalogStore(t *testing.T) *SQLMetadata {
	t.Helper()
	store, openErr := NewSQLMetadata(filepath.Join(t.TempDir(), "metadata.db"), slog.Default())
	if openErr != nil {
		t.Fatalf("new sqlite metadata: %v", openErr)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("close sqlite metadata: %v", closeErr)
		}
	})
	return store
}

func createTestBucket(ctx context.Context, t *testing.T, store *SQLMetadata, bucket string) {
	t.Helper()
	if createErr := store.CreateBucket(ctx, bucket); createErr != nil {
		t.Fatalf("create bucket: %v", createErr)
	}
}

func retainTestDigestRef(ctx context.Context, t *testing.T, store *SQLMetadata) model.DigestRef {
	t.Helper()
	digest, retainErr := store.RetainDigestRef(ctx, model.DigestRef{
		Digest:         "sha256:abc",
		Size:           42,
		UpstreamID:     "primary",
		UpstreamBucket: "raw",
		UpstreamKey:    "sha256/abc",
	})
	if retainErr != nil {
		t.Fatalf("retain digest ref: %v", retainErr)
	}
	if digest.RefCount != 1 {
		t.Fatalf("digest ref count = %d, want 1", digest.RefCount)
	}
	return digest
}

func upsertTestObjectVersion(
	ctx context.Context,
	t *testing.T,
	store *SQLMetadata,
	digest model.DigestRef,
) model.ObjectVersion {
	t.Helper()
	version, upsertErr := store.UpsertObjectVersion(ctx, model.ObjectVersion{
		Bucket:         "photos",
		Key:            "cat.jpg",
		VersionID:      "v1",
		Digest:         digest.Digest,
		ETag:           "etag-v1",
		Size:           digest.Size,
		ContentType:    "image/jpeg",
		UserMetadata:   map[string]string{"camera": "test"},
		UpstreamID:     digest.UpstreamID,
		UpstreamBucket: digest.UpstreamBucket,
		UpstreamKey:    digest.UpstreamKey,
	})
	if upsertErr != nil {
		t.Fatalf("upsert object version: %v", upsertErr)
	}
	if version.VersionID != "v1" || version.UserMetadata["camera"] != "test" {
		t.Fatalf("stored version = %#v", version)
	}
	return version
}

func upsertTestObjectRecord(ctx context.Context, t *testing.T, store *SQLMetadata, version model.ObjectVersion) {
	t.Helper()
	record, upsertErr := store.UpsertObjectRecord(ctx, model.ObjectRecord{
		Bucket:           version.Bucket,
		Key:              version.Key,
		CurrentVersionID: version.VersionID,
	})
	if upsertErr != nil {
		t.Fatalf("upsert object record: %v", upsertErr)
	}
	if record.CurrentVersionID != version.VersionID {
		t.Fatalf("record current version = %q, want %s", record.CurrentVersionID, version.VersionID)
	}
}

func assertTestObjectVersions(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	versions, listErr := store.ListObjectVersions(ctx, "photos", "cat.jpg")
	if listErr != nil {
		t.Fatalf("list object versions: %v", listErr)
	}
	if versions.Len() != 1 {
		t.Fatalf("version count = %d, want 1", versions.Len())
	}
}

func upsertTestIndexDocument(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	document, upsertErr := store.UpsertIndexDocument(ctx, model.IndexDocument{
		ID:        "doc-1",
		Bucket:    "photos",
		Key:       "cat.jpg",
		VersionID: "v1",
		Digest:    "sha256:abc",
		State:     model.IndexDocumentStatePending,
	})
	if upsertErr != nil {
		t.Fatalf("upsert index document: %v", upsertErr)
	}
	if document.State != model.IndexDocumentStatePending {
		t.Fatalf("index document state = %q", document.State)
	}
}

func upsertTestIndexJob(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	job, upsertErr := store.UpsertIndexJob(ctx, model.IndexJob{
		ID:        "job-1",
		Kind:      model.IndexJobKindUpsert,
		Bucket:    "photos",
		Key:       "cat.jpg",
		VersionID: "v1",
	})
	if upsertErr != nil {
		t.Fatalf("upsert index job: %v", upsertErr)
	}
	if job.Status != model.IndexJobStatusQueued {
		t.Fatalf("index job status = %q", job.Status)
	}
	assertQueuedIndexJobs(ctx, t, store)
}

func assertQueuedIndexJobs(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	jobs, listErr := store.ListIndexJobs(ctx, model.IndexJobStatusQueued, 10)
	if listErr != nil {
		t.Fatalf("list index jobs: %v", listErr)
	}
	if jobs.Len() != 1 {
		t.Fatalf("queued job count = %d, want 1", jobs.Len())
	}
}

func upsertTestIndexOutboxEvent(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	event, upsertErr := store.UpsertIndexOutboxEvent(ctx, model.IndexOutboxEvent{
		ID:        "event-1",
		EventType: "index.document.upserted",
		Bucket:    "photos",
		Key:       "cat.jpg",
		VersionID: "v1",
		Payload:   `{"document_id":"doc-1"}`,
	})
	if upsertErr != nil {
		t.Fatalf("upsert index outbox event: %v", upsertErr)
	}
	if event.Status != model.IndexOutboxStatusPending {
		t.Fatalf("index outbox status = %q", event.Status)
	}
}

func deleteTestCatalogEntries(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	assertDeleteObjectVersion(ctx, t, store)
	assertDeleteObjectRecord(ctx, t, store)
	assertReleaseDigestRef(ctx, t, store)
}

func assertDeleteObjectVersion(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	removed, deleteErr := store.DeleteObjectVersion(ctx, "photos", "cat.jpg", "v1")
	if deleteErr != nil {
		t.Fatalf("delete object version: %v", deleteErr)
	}
	if !removed {
		t.Fatal("expected object version delete")
	}
}

func assertDeleteObjectRecord(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	removed, deleteErr := store.DeleteObjectRecord(ctx, "photos", "cat.jpg")
	if deleteErr != nil {
		t.Fatalf("delete object record: %v", deleteErr)
	}
	if !removed {
		t.Fatal("expected object record delete")
	}
}

func assertReleaseDigestRef(ctx context.Context, t *testing.T, store *SQLMetadata) {
	t.Helper()
	_, removed, releaseErr := store.ReleaseDigestRef(ctx, "sha256:abc")
	if releaseErr != nil {
		t.Fatalf("release digest ref: %v", releaseErr)
	}
	if !removed {
		t.Fatal("expected digest ref removal")
	}
}
