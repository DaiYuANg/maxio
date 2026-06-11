package metadata

import (
	"log/slog"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
)

func TestModuleSelectsInMemoryMetadataWhenClusterDisabled(t *testing.T) {
	cfg := config.Config{
		EnableClusterManagement: false,
	}
	store, err := newMetadataStore(cfg, nil, slog.Default())
	if err != nil {
		t.Fatalf("new metadata store: %v", err)
	}
	if _, ok := store.(*InMemoryMetadata); !ok {
		t.Fatalf("metadata store type = %T, want *InMemoryMetadata", store)
	}
}

func TestModuleRequiresRaftRuntimeWhenClusterEnabled(t *testing.T) {
	cfg := config.Config{
		EnableClusterManagement: true,
	}
	store, err := newMetadataStore(cfg, nil, slog.Default())
	if err == nil {
		t.Fatalf("new metadata store err = nil, want error, store=%v", store)
	}
}
