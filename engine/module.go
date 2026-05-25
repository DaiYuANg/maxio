package engine

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/spf13/afero"
)

// Module returns a dix module for the erasure-coded storage engine.
func Module() dix.Module {
	return dix.NewModule(
		"engine",
		dix.WithModuleProviders(
			dix.ProviderErr2(func(cfg config.Config, logger *slog.Logger) (*Engine, error) {
				dataDir := cfg.DataDir
				if dataDir == "" {
					dataDir = "./data/engine"
				}
				engine, err := NewEngine(dataDir, DefaultDataChunks, DefaultParityChunks, afero.NewOsFs())
				if err != nil {
					return nil, fmt.Errorf("engine init: %w", err)
				}
				storageNodeID := raftStorageNodeID(cfg.RaftNodeID)
				engine.ConfigureLocalNode(storageNodeID, cfg.StorageAdvertiseAddress())
				logger.Info("erasure engine initialized",
					"data_chunks", DefaultDataChunks,
					"parity_chunks", DefaultParityChunks,
					"data_dir", dataDir,
					"storage_node", storageNodeID,
					"storage_address", cfg.StorageAdvertiseAddress(),
				)
				return engine, nil
			}),
		),
		dix.Hooks(
			dix.OnStop(func(_ context.Context, engine *Engine) error {
				if engine == nil {
					return nil
				}
				return nil
			}),
		),
	)
}

// WithDataChunks sets the number of data chunks.
func WithDataChunks(n int) ModuleOption {
	return func(opts *moduleOptions) {
		opts.dataChunks = n
	}
}

// WithParityChunks sets the number of parity chunks.
func WithParityChunks(n int) ModuleOption {
	return func(opts *moduleOptions) {
		opts.parityChunks = n
	}
}

// ModuleOption configures the engine module.
type ModuleOption func(*moduleOptions)

type moduleOptions struct {
	dataChunks   int
	parityChunks int
}
