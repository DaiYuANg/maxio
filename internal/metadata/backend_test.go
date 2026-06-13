package metadata

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
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
