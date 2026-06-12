package metadata

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/config"
)

func Module() dix.Module {
	return dix.NewModule("metadata",
		dix.WithModuleProviders(
			dix.ProviderErr2(newMetadataStore),
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

func newMetadataStore(cfg config.Config, logger *slog.Logger) (MetadataStore, error) {
	if logger == nil {
		logger = slog.Default()
	}

	store, err := NewSQLiteMetadata(filepath.Join(cfg.DataDir, "metadata.db"), logger)
	if err != nil {
		return nil, fmt.Errorf("metadata backend: %w", err)
	}

	logger.Info("metadata backend selected", "backend", "sqlite")
	return store, nil
}
