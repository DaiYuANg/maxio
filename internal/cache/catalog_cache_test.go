package cache

import (
	"context"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestMemoryMetadataCacheCatalogRoundTrip(t *testing.T) {
	ctx := context.Background()
	cache := newTestMemoryCache(t, time.Minute)

	version := testObjectVersion()
	if err := cache.SetObjectVersion(ctx, version); err != nil {
		t.Fatalf("set object version: %v", err)
	}
	cache.Wait()
	gotVersion, found, err := cache.GetObjectVersion(ctx, version.Bucket, version.Key)
	if err != nil {
		t.Fatalf("get object version: %v", err)
	}
	if !found || gotVersion.VersionID != version.VersionID {
		t.Fatalf("expected cached object version %q, found=%v got=%q", version.VersionID, found, gotVersion.VersionID)
	}

	ref := testDigestRef()
	if setDigestErr := cache.SetDigestRef(ctx, ref); setDigestErr != nil {
		t.Fatalf("set digest ref: %v", setDigestErr)
	}
	cache.Wait()
	gotRef, found, err := cache.GetDigestRef(ctx, ref.Digest)
	if err != nil {
		t.Fatalf("get digest ref: %v", err)
	}
	if !found || gotRef.UpstreamKey != ref.UpstreamKey {
		t.Fatalf("expected cached digest ref %q, found=%v got=%q", ref.UpstreamKey, found, gotRef.UpstreamKey)
	}
}

func TestRedisMetadataCacheCatalogRoundTrip(t *testing.T) {
	ctx := context.Background()
	client := newFakeRedisClient()
	cache := NewRedisCache(client, WithRedisPrefix("test-cache"), WithRedisTTL(time.Minute))

	version := testObjectVersion()
	if err := cache.SetObjectVersion(ctx, version); err != nil {
		t.Fatalf("set object version: %v", err)
	}
	gotVersion, found, err := cache.GetObjectVersion(ctx, version.Bucket, version.Key)
	if err != nil {
		t.Fatalf("get object version: %v", err)
	}
	if !found || gotVersion.VersionID != version.VersionID {
		t.Fatalf("expected cached object version %q, found=%v got=%q", version.VersionID, found, gotVersion.VersionID)
	}

	ref := testDigestRef()
	if setDigestErr := cache.SetDigestRef(ctx, ref); setDigestErr != nil {
		t.Fatalf("set digest ref: %v", setDigestErr)
	}
	gotRef, found, err := cache.GetDigestRef(ctx, ref.Digest)
	if err != nil {
		t.Fatalf("get digest ref: %v", err)
	}
	if !found || gotRef.UpstreamKey != ref.UpstreamKey {
		t.Fatalf("expected cached digest ref %q, found=%v got=%q", ref.UpstreamKey, found, gotRef.UpstreamKey)
	}
}

func testObjectVersion() model.ObjectVersion {
	return model.ObjectVersion{
		Bucket:         "photos",
		Key:            "cat.jpg",
		VersionID:      "v1",
		Digest:         "sha256:abc",
		Size:           3,
		UpstreamID:     "up-1",
		UpstreamBucket: "photos",
		UpstreamKey:    "canonical.jpg",
		UserMetadata:   map[string]string{"kind": "cat"},
	}
}

func testDigestRef() model.DigestRef {
	return model.DigestRef{
		Digest:         "sha256:abc",
		Size:           3,
		RefCount:       1,
		UpstreamID:     "up-1",
		UpstreamBucket: "photos",
		UpstreamKey:    "canonical.jpg",
	}
}
