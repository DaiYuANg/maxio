// Package object provides MaxIO's internal object service API.
package object

import (
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/index"
)

func Module() dix.Module {
	return dix.NewModule(
		"object",
		dix.WithModuleProviders(
			dix.Provider4(newServiceFromRuntimeConfig),
		),
	)
}

func newServiceFromRuntimeConfig(
	search *index.SearchEngine,
	bus eventx.BusRuntime,
	logger *slog.Logger,
	cfg config.Config,
) *Service {
	return NewService(nil, search, bus, logger, objectConfigFromRuntime(cfg))
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
