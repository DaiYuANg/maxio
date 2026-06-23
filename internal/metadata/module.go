package metadata

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/scheduler"
)

func Module() dix.Module {
	return dix.NewModule("metadata",
		dix.WithModuleProviders(
			dix.ProviderErr2(newMetadataStore),
			dix.ProviderErr1(newSchedulerLeaseRepository),
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

	store, err := NewMetadataStore(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("metadata backend: %w", err)
	}

	logger.Info("metadata backend selected", "backend", cfg.MetadataBackend)
	return store, nil
}

func newSchedulerLeaseRepository(store MetadataStore) (scheduler.LeaseRepository, error) {
	sqlStore, ok := store.(*SQLMetadata)
	if !ok {
		return nil, fmt.Errorf("scheduler lease repository requires SQL metadata backend, got %T", store)
	}
	return NewSQLSchedulerLeaseRepository(sqlStore), nil
}
