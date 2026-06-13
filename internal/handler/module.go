// Package handler provides MaxIO HTTP route handlers.
package handler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/discovery"
	"github.com/lyonbrown4d/maxio/internal/metadata"
)

func Module() dix.Module {
	return dix.NewModule(
		"infra",
		dix.WithModuleProviders(
			dix.Provider1(newLogger),
			dix.Provider1(newEventBus),
			dix.Provider2(newGatewayDependencies),
			dix.Provider3(newGatewayService),
		),
		dix.Hooks(
			dix.OnStop(closeEventBus),
		),
	)
}

func newEventBus(logger *slog.Logger) eventx.BusRuntime {
	return eventx.New(eventx.WithMiddleware(busMiddleware(logger)))
}

func newGatewayDependencies(discoveryRuntime *discovery.Runtime, metadataStore metadata.MetadataStore) Dependencies {
	return NewDependencies(nil, nil, discoveryRuntime, nil, metadataStore)
}

func newGatewayService(deps Dependencies, logger *slog.Logger, cfg config.Config) *Service {
	return newService(deps, logger, cfg, nil)
}

func closeEventBus(_ context.Context, bus eventx.BusRuntime) error {
	if err := bus.Close(); err != nil {
		return fmt.Errorf("close event bus: %w", err)
	}
	return nil
}

func busMiddleware(logger *slog.Logger) eventx.Middleware {
	return func(handlerFn eventx.HandlerFunc) eventx.HandlerFunc {
		return func(ctx context.Context, event eventx.Event) error {
			if err := handlerFn(ctx, event); err != nil {
				logger.ErrorContext(ctx, "event bus handler error", "event", event.Name(), "error", err)
				return err
			}
			return nil
		}
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	level, err := logx.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = slog.LevelInfo
	}

	logger, err := logx.New(
		logx.WithLevel(level),
		logx.WithCaller(true),
		logx.WithGlobalLogger(),
	)
	if err == nil {
		return logger
	}
	return slog.Default()
}
