package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestDedupeMiddlewarePutHitShortCircuitsUpstream(t *testing.T) {
	ctx := context.Background()
	store := metadata.NewInMemoryMetadata()
	body := []byte("hello")
	digest := testDigest(body)
	seedDigestRef(ctx, t, store, model.DigestRef{
		Digest:         digest,
		Size:           int64(len(body)),
		RefCount:       1,
		UpstreamID:     "up",
		UpstreamBucket: "photos",
		UpstreamKey:    "existing.txt",
	})

	middleware := &dedupeMiddleware{store: store, logger: slog.Default()}
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})
	req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/photos/copy.txt", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	middleware.handlePut(next, "up", recorder, req)

	if called {
		t.Fatal("expected dedupe hit to skip upstream")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	if got := recorder.Header().Get("X-Maxio-Dedupe"); got != "hit" {
		t.Fatalf("expected dedupe hit header, got %q", got)
	}
	version := requireCurrentVersion(ctx, t, store, "photos", "copy.txt")
	if version.UpstreamKey != "existing.txt" {
		t.Fatalf("expected canonical upstream key existing.txt, got %q", version.UpstreamKey)
	}
	ref := requireDigestRef(ctx, t, store, digest)
	if ref.RefCount != 2 {
		t.Fatalf("expected refcount 2, got %d", ref.RefCount)
	}
}

func TestDedupeMiddlewarePutMissForwardsAndCommitsMetadata(t *testing.T) {
	ctx := context.Background()
	store := metadata.NewInMemoryMetadata()
	body := []byte("new object")
	digest := testDigest(body)

	middleware := &dedupeMiddleware{store: store, logger: slog.Default()}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		received, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read replayed body: %v", err)
		}
		if !bytes.Equal(received, body) {
			t.Fatalf("unexpected replayed body %q", string(received))
		}
		w.Header().Set("ETag", `"upstream-etag"`)
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequestWithContext(ctx, http.MethodPut, "/photos/new.txt", bytes.NewReader(body))
	recorder := httptest.NewRecorder()

	middleware.handlePut(next, "up", recorder, req)

	if !called {
		t.Fatal("expected dedupe miss to forward upstream")
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	version := requireCurrentVersion(ctx, t, store, "photos", "new.txt")
	if version.Digest != digest {
		t.Fatalf("expected digest %q, got %q", digest, version.Digest)
	}
	if version.UpstreamKey != "new.txt" {
		t.Fatalf("expected upstream key new.txt, got %q", version.UpstreamKey)
	}
	ref := requireDigestRef(ctx, t, store, digest)
	if ref.RefCount != 1 {
		t.Fatalf("expected refcount 1, got %d", ref.RefCount)
	}
}

func TestDedupeMiddlewareReadRewritesToCanonicalObject(t *testing.T) {
	ctx := context.Background()
	store := metadata.NewInMemoryMetadata()
	seedObjectVersion(ctx, t, store, model.ObjectVersion{
		Bucket:         "photos",
		Key:            "copy.txt",
		VersionID:      "v1",
		Digest:         testDigest([]byte("hello")),
		Size:           5,
		UpstreamID:     "up",
		UpstreamBucket: "photos",
		UpstreamKey:    "existing.txt",
	})

	middleware := &dedupeMiddleware{store: store, logger: slog.Default()}
	var rewrittenPath string
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		rewrittenPath = r.URL.Path
	})
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/photos/copy.txt", http.NoBody)
	recorder := httptest.NewRecorder()

	middleware.handleRead(next, "up", recorder, req)

	if rewrittenPath != "/photos/existing.txt" {
		t.Fatalf("expected rewritten path /photos/existing.txt, got %q", rewrittenPath)
	}
}

func testDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func seedDigestRef(ctx context.Context, t *testing.T, store metadata.MetadataStore, ref model.DigestRef) {
	t.Helper()
	if _, err := store.RetainDigestRef(ctx, ref); err != nil {
		t.Fatalf("retain digest ref: %v", err)
	}
}

func seedObjectVersion(ctx context.Context, t *testing.T, store metadata.MetadataStore, version model.ObjectVersion) {
	t.Helper()
	if err := store.CreateBucket(ctx, version.Bucket); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	stored, err := store.UpsertObjectVersion(ctx, version)
	if err != nil {
		t.Fatalf("upsert object version: %v", err)
	}
	if _, err := store.UpsertObjectRecord(ctx, model.ObjectRecord{
		Bucket:           version.Bucket,
		Key:              version.Key,
		CurrentVersionID: stored.VersionID,
	}); err != nil {
		t.Fatalf("upsert object record: %v", err)
	}
}

func requireCurrentVersion(
	ctx context.Context,
	t *testing.T,
	store metadata.MetadataStore,
	bucket string,
	key string,
) model.ObjectVersion {
	t.Helper()
	record, found, err := store.GetObjectRecord(ctx, bucket, key)
	if err != nil {
		t.Fatalf("get object record: %v", err)
	}
	if !found {
		t.Fatalf("expected object record %s/%s", bucket, key)
	}
	version, found, err := store.GetObjectVersion(ctx, bucket, key, record.CurrentVersionID)
	if err != nil {
		t.Fatalf("get object version: %v", err)
	}
	if !found {
		t.Fatalf("expected object version %s/%s/%s", bucket, key, record.CurrentVersionID)
	}
	return version
}

func requireDigestRef(ctx context.Context, t *testing.T, store metadata.MetadataStore, digest string) model.DigestRef {
	t.Helper()
	ref, found, err := store.GetDigestRef(ctx, digest)
	if err != nil {
		t.Fatalf("get digest ref: %v", err)
	}
	if !found {
		t.Fatalf("expected digest ref %s", digest)
	}
	return ref
}
