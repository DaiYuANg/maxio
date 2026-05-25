package store

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/metadata"
)

func Module() dix.Module {
	return dix.NewModule(
		"store",
		dix.WithModuleProviders(
			dix.ProviderErr4(func(
				cfg config.Config,
				metaStore metadata.MetadataStore,
				engineStore *engine.Engine,
				logger *slog.Logger,
			) (*Store, error) {
				store, err := NewStore(cfg.DataDir, metaStore, engineStore)
				if err != nil {
					return nil, fmt.Errorf("store init: %w", err)
				}
				logger.Info("store initialized",
					"backend", "erasure",
					"data_dir", cfg.DataDir,
				)
				return store, nil
			}),
			dix.Provider3(func(cfg config.Config, store *Store, logger *slog.Logger) *pendingCleanup {
				return &pendingCleanup{
					cfg:    cfg,
					store:  store,
					logger: logger,
				}
			}),
		),
		dix.Hooks(
			dix.OnStart(func(ctx context.Context, cleanup *pendingCleanup) error {
				if cleanup == nil {
					return nil
				}
				return cleanup.run(ctx)
			}),
			dix.OnStop(func(_ context.Context, store *Store) error {
				if store == nil {
					return nil
				}
				if store.meta == nil {
					return nil
				}
				if closer, ok := store.meta.(interface{ Close() error }); ok {
					return closer.Close()
				}
				return nil
			}),
		),
	)
}
