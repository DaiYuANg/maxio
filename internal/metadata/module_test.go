package metadata

import (
	"log/slog"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
)

func TestModuleSelectsSQLiteMetadataWhenClusterDisabled(t *testing.T) {
	cfg := config.Config{
		EnableClusterManagement: false,
		DataDir:                 t.TempDir(),
	}
	store, err := newMetadataStore(cfg, slog.Default())
	if err != nil {
		t.Fatalf("new metadata store: %v", err)
	}
	sqliteStore, ok := store.(*SQLiteMetadata)
	if !ok {
		t.Fatalf("metadata store type = %T, want *SQLiteMetadata", store)
	}
	if err := sqliteStore.Close(); err != nil {
		t.Fatalf("close sqlite metadata: %v", err)
	}
}

func TestModuleUsesSQLiteMetadataWhenLegacyClusterFlagIsEnabled(t *testing.T) {
	cfg := config.Config{
		EnableClusterManagement: true,
		DataDir:                 t.TempDir(),
	}
	store, err := newMetadataStore(cfg, slog.Default())
	if err != nil {
		t.Fatalf("new metadata store: %v", err)
	}
	sqliteStore, ok := store.(*SQLiteMetadata)
	if !ok {
		t.Fatalf("metadata store type = %T, want *SQLiteMetadata", store)
	}
	if err := sqliteStore.Close(); err != nil {
		t.Fatalf("close sqlite metadata: %v", err)
	}
}
