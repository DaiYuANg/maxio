// Package handler provides MaxIO HTTP route handlers.
package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/logx"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/object"
)

func Module() dix.Module {
	return dix.NewModule(
		"infra",
		dix.WithModuleProviders(
			dix.Provider1(newLogger),
			dix.Provider1(newEventBus),
			dix.Provider4(NewDependencies),
			dix.Provider4(newService),
		),
		dix.Hooks(
			dix.OnStart2(startObjectEventSubscription),
			dix.OnStart3(syncStorageNodesOnStart),
			dix.OnStop(closeEventBus),
		),
	)
}

func newEventBus(logger *slog.Logger) eventx.BusRuntime {
	return eventx.New(eventx.WithMiddleware(busMiddleware(logger)))
}

func startObjectEventSubscription(_ context.Context, bus eventx.BusRuntime, logger *slog.Logger) error {
	_, err := eventx.Subscribe(bus, func(_ context.Context, event object.ObjectEvent) error {
		logger.Info("object event",
			"event", event.Payload["event"],
			"bucket", event.Payload["bucket"],
			"key", event.Payload["key"],
		)
		return nil
	})
	if err != nil {
		return fmt.Errorf("subscribe object event failed: %w", err)
	}
	return nil
}

func syncStorageNodesOnStart(ctx context.Context, service *Service, _ eventx.BusRuntime, logger *slog.Logger) error {
	if service == nil {
		return nil
	}
	if !service.cfg.EnableClusterManagement {
		if logger != nil {
			logger.InfoContext(ctx, "skip raft storage node sync because cluster management is disabled")
		}
		return nil
	}
	if logger != nil {
		logger.InfoContext(ctx, "syncing storage nodes from raft membership")
	}
	if err := syncStorageNodesOnStartWithRetry(ctx, service, logger); err != nil {
		return fmt.Errorf("sync storage nodes from raft failed: %w", err)
	}
	if logger != nil {
		logger.InfoContext(ctx, "storage nodes synced from raft")
	}
	return nil
}

func syncStorageNodesOnStartWithRetry(ctx context.Context, service *Service, logger *slog.Logger) error {
	const attempts = 40
	const delay = 250 * time.Millisecond

	var lastErr error
	for attempt := range attempts {
		err := service.syncStorageNodes(ctx)
		if err == nil {
			return nil
		}
		lastErr = err
		if !shouldRetryStorageNodeSync(err, attempt, attempts) {
			break
		}
		logStorageNodeSyncRetry(ctx, logger, err, attempt, attempts, delay)
		if waitErr := waitStorageNodeSyncRetry(ctx, delay); waitErr != nil {
			return waitErr
		}
	}
	return lastErr
}

func shouldRetryStorageNodeSync(err error, attempt, attempts int) bool {
	return isTransientRaftMembershipError(err) && attempt < attempts-1
}

func logStorageNodeSyncRetry(
	ctx context.Context,
	logger *slog.Logger,
	err error,
	attempt int,
	attempts int,
	delay time.Duration,
) {
	if logger == nil {
		return
	}
	logger.DebugContext(ctx, "retry storage node sync while raft shard is becoming ready",
		"attempt", attempt+1,
		"max_attempts", attempts,
		"delay", delay.String(),
		"error", err,
	)
}

func isTransientRaftMembershipError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "shard is not ready") ||
		strings.Contains(message, "request dropped") ||
		strings.Contains(message, "leader unavailable") ||
		strings.Contains(message, "not ready")
}

func waitStorageNodeSyncRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wait storage node sync retry: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
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
