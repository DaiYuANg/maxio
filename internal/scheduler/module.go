// Package scheduler provides a simple wrapper around gocron scheduling.
package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/arcgolabs/dix"
	gocron "github.com/go-co-op/gocron/v2"
)

// Runtime wraps gocron with MaxIO lifecycle and lease-based task scheduling.
type Runtime struct {
	scheduler       gocron.Scheduler
	logger          *slog.Logger
	leaseRepository LeaseRepository
	leaseOwner      string
	leaseSequence   uint64
}

func Module() dix.Module {
	return dix.NewModule(
		"scheduler",
		dix.WithModuleProviders(
			dix.ProviderErr1(newRuntime),
		),
		dix.Hooks(
			dix.OnStart(startRuntime),
			dix.OnStop(stopRuntime),
		),
	)
}

func newRuntime(
	logger *slog.Logger,
) (*Runtime, error) {
	if logger == nil {
		logger = slog.Default()
	}

	schedulerOptions := make([]gocron.SchedulerOption, 0, 2)
	schedulerOptions = append(schedulerOptions,
		gocron.WithLogger(slogCronLogger{logger: logger.With("component", "gocron")}),
		gocron.WithStopTimeout(10*time.Second),
	)

	schedulerRuntime, err := gocron.NewScheduler(schedulerOptions...)
	if err != nil {
		return nil, fmt.Errorf("create scheduler: %w", err)
	}

	return &Runtime{
		scheduler:       schedulerRuntime,
		logger:          logger,
		leaseRepository: NewInMemoryLeaseRepository(),
		leaseOwner:      defaultLeaseOwner(),
	}, nil
}

func startRuntime(ctx context.Context, runtime *Runtime) error {
	if runtime == nil || runtime.scheduler == nil {
		return nil
	}
	runtime.scheduler.Start()
	if runtime.logger != nil {
		runtime.logger.InfoContext(ctx, "scheduler started")
	}
	return nil
}

func stopRuntime(ctx context.Context, runtime *Runtime) error {
	if runtime == nil || runtime.scheduler == nil {
		return nil
	}
	if err := runtime.scheduler.ShutdownWithContext(ctx); err != nil {
		return fmt.Errorf("shutdown scheduler: %w", err)
	}
	if runtime.logger != nil {
		runtime.logger.InfoContext(ctx, "scheduler stopped")
	}
	return nil
}

func (runtime *Runtime) NewJob(definition gocron.JobDefinition, task gocron.Task, options ...gocron.JobOption) (gocron.Job, error) {
	if runtime == nil || runtime.scheduler == nil {
		return nil, errors.New("scheduler unavailable")
	}
	job, err := runtime.scheduler.NewJob(definition, task, options...)
	if err != nil {
		return nil, fmt.Errorf("create scheduled job: %w", err)
	}
	return job, nil
}

type slogCronLogger struct {
	logger *slog.Logger
}

func (logger slogCronLogger) Debug(message string, args ...any) {
	logger.log().Debug(message, args...)
}

func (logger slogCronLogger) Error(message string, args ...any) {
	logger.log().Error(message, args...)
}

func (logger slogCronLogger) Info(message string, args ...any) {
	logger.log().Info(message, args...)
}

func (logger slogCronLogger) Warn(message string, args ...any) {
	logger.log().Warn(message, args...)
}

func (logger slogCronLogger) log() *slog.Logger {
	if logger.logger == nil {
		return slog.Default()
	}
	return logger.logger
}
