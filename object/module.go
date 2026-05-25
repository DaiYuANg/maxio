// Package object exposes MaxIO's public object service API.
package object

import (
	"context"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/maxio/index"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/store"
)

func Module() dix.Module {
	return dix.NewModule(
		"object",
		dix.WithModuleProviders(
			dix.Provider5(newServiceFromRuntimeConfig),
		),
		dix.Hooks(
			dix.OnStart(func(ctx context.Context, service *Service) error {
				return service.StartIndexWorker(ctx)
			}),
			dix.OnStop(func(_ context.Context, service *Service) error {
				return service.stopIndexWorker()
			}),
		),
	)
}

func newServiceFromRuntimeConfig(
	storage *store.Store,
	search *index.SearchEngine,
	bus eventx.BusRuntime,
	logger *slog.Logger,
	cfg config.Config,
) *Service {
	return NewService(storage, search, bus, logger, objectConfigFromRuntime(cfg))
}

func objectConfigFromRuntime(cfg config.Config) Config {
	return Config{
		DedupeMaxFixes:    cfg.DedupeMaxFixes,
		PendingObjectTTL:  cfg.PendingObjectTTLDuration(),
		IndexTimeout:      cfg.IndexTimeoutDuration(),
		IndexRetryBackoff: cfg.IndexRetryBackoffDuration(),
		IndexMaxRetries:   cfg.IndexMaxRetries,
		IndexQueueSize:    cfg.IndexQueueSize,
		IndexRateLimit:    cfg.IndexRateLimit,
	}
}
