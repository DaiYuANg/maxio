package metadata

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/config"
	raftx "github.com/lyonbrown4d/maxio/internal/raft"
)

func Module() dix.Module {
	return dix.NewModule("metadata",
		dix.WithModuleProviders(
			dix.ProviderErr3(newMetadataStore),
		),
		dix.Hooks(
			dix.OnStop(func(_ context.Context, store MetadataStore) error {
				if closer, ok := store.(interface{ Close() error }); ok {
					return closer.Close()
				}
				return nil
			}),
		),
	)
}

func newMetadataStore(cfg config.Config, runtime *raftx.Runtime, logger *slog.Logger) (MetadataStore, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.EnableClusterManagement {
		store, err := NewRaftMetadata(runtime)
		if err != nil {
			return nil, fmt.Errorf("metadata backend: %w", err)
		}
		logger.Info("metadata backend selected", "backend", "raft")
		return store, nil
	}

	logger.Info("metadata backend selected", "backend", "memory")
	return NewInMemoryMetadata(), nil
}
