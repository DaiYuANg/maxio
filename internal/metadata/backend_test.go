package metadata

import (
	"context"
	"log/slog"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/model"
)

func TestNewMetadataStoreSupportsSQLite(t *testing.T) {
	cfg := config.Config{
		DataDir:         t.TempDir(),
		MetadataBackend: "sqlite",
	}
	store, err := NewMetadataStore(cfg, slog.Default())
	if err != nil {
		t.Fatalf("new metadata store: %v", err)
	}
	sqliteStore, ok := store.(*SQLiteMetadata)
	if !ok {
		t.Fatalf("unexpected metadata store type: %T", store)
	}
	if closeErr := sqliteStore.Close(); closeErr != nil {
		t.Fatalf("close metadata store: %v", closeErr)
	}
}

func TestNewMetadataStoreRejectsUnsupportedBackend(t *testing.T) {
	cfg := config.Config{
		DataDir:         t.TempDir(),
		MetadataBackend: "unsupported",
	}
	_, err := NewMetadataStore(cfg, slog.Default())
	if err == nil {
		t.Fatal("expected error for unsupported backend")
	}
	if !strings.Contains(err.Error(), "unsupported metadata backend") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSQLiteMetadataUpstreamLifecycle(t *testing.T) {
	store, err := NewSQLiteMetadata(filepath.Join(t.TempDir(), "metadata.db"), slog.Default())
	if err != nil {
		t.Fatalf("new sqlite metadata: %v", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Fatalf("close sqlite metadata: %v", closeErr)
		}
	}()

	stored := mustUpsertTestUpstream(t, store)
	assertStoredTestUpstream(t, stored)
	mustGetTestUpstream(t, store)
	mustListTestUpstream(t, store)
	mustDeleteTestUpstream(t, store)
}

func testUpstream() model.Upstream {
	return model.Upstream{
		ID:       "u-1",
		Name:     "primary",
		Endpoint: "http://127.0.0.1:9000",
		Region:   "local",
		Weight:   2,
		Priority: 10,
		Buckets:  []string{"photos", "docs", "photos"},
		Enabled:  true,
	}
}

func mustUpsertTestUpstream(t *testing.T, store *SQLiteMetadata) model.Upstream {
	t.Helper()

	upstream := testUpstream()
	stored, err := store.UpsertUpstream(context.Background(), upstream)
	if err != nil {
		t.Fatalf("upsert upstream: %v", err)
	}
	return stored
}

func assertStoredTestUpstream(t *testing.T, stored model.Upstream) {
	t.Helper()

	upstream := testUpstream()
	if stored.ID != upstream.ID || stored.Name != upstream.Name || stored.Endpoint != upstream.Endpoint {
		t.Fatalf("stored upstream = %#v", stored)
	}
	if !reflect.DeepEqual(stored.Buckets, []string{"photos", "docs"}) {
		t.Fatalf("stored buckets = %#v", stored.Buckets)
	}
}

func mustGetTestUpstream(t *testing.T, store *SQLiteMetadata) {
	t.Helper()

	found, ok, err := store.GetUpstream(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("get upstream: %v", err)
	}
	if !ok || found.ID != "u-1" {
		t.Fatalf("found=%v upstream=%#v", ok, found)
	}
}

func mustListTestUpstream(t *testing.T, store *SQLiteMetadata) {
	t.Helper()

	upstreams, err := store.ListUpstreams(context.Background())
	if err != nil {
		t.Fatalf("list upstreams: %v", err)
	}
	if len(upstreams) != 1 {
		t.Fatalf("upstreams count = %d, want 1", len(upstreams))
	}
}

func mustDeleteTestUpstream(t *testing.T, store *SQLiteMetadata) {
	t.Helper()

	deleted, err := store.DeleteUpstream(context.Background(), "u-1")
	if err != nil {
		t.Fatalf("delete upstream: %v", err)
	}
	if !deleted {
		t.Fatal("expected upstream to be deleted")
	}
}
