package metadata

import (
	"log/slog"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
)

func TestModuleSelectsSQLMetadata(t *testing.T) {
	cfg := config.Config{
		DataDir: t.TempDir(),
	}
	store, err := newMetadataStore(cfg, slog.Default())
	if err != nil {
		t.Fatalf("new metadata store: %v", err)
	}
	sqlStore, ok := store.(*SQLMetadata)
	if !ok {
		t.Fatalf("metadata store type = %T, want *SQLMetadata", store)
	}
	if err := sqlStore.Close(); err != nil {
		t.Fatalf("close sqlite metadata: %v", err)
	}
}
