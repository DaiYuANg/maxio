// Package index provides an object metadata search module backed by Bleve.
package index

import (
	"context"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/config"
)

func Module() dix.Module {
	return dix.NewModule(
		"index",
		dix.WithModuleProviders(
			dix.ProviderErr2(func(cfg config.Config, logger *slog.Logger) (*SearchEngine, error) {
				return NewSearchEngine(Config{DataDir: cfg.DataDir}, logger)
			}),
		),
		dix.Hooks(
			dix.OnStop(func(_ context.Context, search *SearchEngine) error {
				return search.Close()
			}),
		),
	)
}
