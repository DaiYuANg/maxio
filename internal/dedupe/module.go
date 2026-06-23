// Package dedupe schedules background object-level dedupe reconciliation.
package dedupe

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arcgolabs/dix"
	gocron "github.com/go-co-op/gocron/v2"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/object"
	"github.com/lyonbrown4d/maxio/internal/scheduler"
)

const dedupeJobName = "maxio.object.dedupe"

type Status struct {
	Running        bool                `json:"running"`
	LastStartedAt  time.Time           `json:"last_started_at,omitzero"`
	LastFinishedAt time.Time           `json:"last_finished_at,omitzero"`
	LastError      string              `json:"last_error,omitempty"`
	LastResult     object.DedupeResult `json:"last_result"`
}

type Runtime struct {
	cfg       config.Config
	objects   *object.Service
	scheduler *scheduler.Runtime
	logger    *slog.Logger
	mu        sync.RWMutex
	status    Status
}

func Module() dix.Module {
	return dix.NewModule(
		"dedupe",
		dix.WithModuleProviders(
			dix.Provider4(NewRuntime),
		),
		dix.Hooks(
			dix.OnStart(startRuntime),
		),
	)
}

func NewRuntime(
	cfg config.Config,
	objects *object.Service,
	schedulerRuntime *scheduler.Runtime,
	logger *slog.Logger,
) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{
		cfg:       cfg,
		objects:   objects,
		scheduler: schedulerRuntime,
		logger:    logger,
	}
}

func startRuntime(ctx context.Context, runtime *Runtime) error {
	if runtime == nil {
		return nil
	}
	return runtime.Start(ctx)
}

func (runtime *Runtime) Start(ctx context.Context) error {
	interval := runtime.cfg.DedupeIntervalDuration()
	if _, err := runtime.scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(runtime.runScheduled),
		gocron.WithName(dedupeJobName),
		gocron.WithTags("dedupe", "storage"),
	); err != nil {
		return fmt.Errorf("schedule dedupe job: %w", err)
	}
	runtime.logger.InfoContext(ctx, "object dedupe job scheduled",
		"job", dedupeJobName,
		"interval", interval.String(),
		"max_fixes", runtime.cfg.DedupeMaxFixes,
	)
	if runtime.cfg.DedupeOnStart {
		runtime.startInitialDedupe(ctx)
	}
	return nil
}

func (runtime *Runtime) startInitialDedupe(ctx context.Context) {
	if ctx == nil {
		runtime.logger.Warn("skip initial dedupe: nil context")
		return
	}
	go func() {
		runtime.runScheduled(ctx)
	}()
}

func (runtime *Runtime) runScheduled(ctx context.Context) {
	result, err := runtime.RunOnce(ctx)
	if err != nil {
		runtime.logger.ErrorContext(ctx, "object dedupe job failed", "error", err)
		return
	}
	runtime.logger.DebugContext(ctx, "object dedupe job completed",
		"objects", result.Objects,
		"blob_refs", result.BlobRefs,
		"fixes", result.Fixes,
		"limited", result.Limited,
	)
}

func (runtime *Runtime) RunOnce(ctx context.Context) (object.DedupeResult, error) {
	if runtime == nil {
		return object.DedupeResult{}, errors.New("dedupe runtime unavailable")
	}
	runtime.markStarted()
	result, err := runtime.runOnce(ctx)
	runtime.markFinished(result, err)
	return result, err
}

func (runtime *Runtime) PlanOnce(ctx context.Context) (object.DedupeResult, error) {
	if runtime == nil || runtime.objects == nil {
		return object.DedupeResult{}, errors.New("dedupe runtime unavailable")
	}
	result, err := runtime.objects.PlanDedupe(ctx)
	if err != nil {
		return object.DedupeResult{}, fmt.Errorf("plan object dedupe: %w", err)
	}
	return result, nil
}

func (runtime *Runtime) runOnce(ctx context.Context) (object.DedupeResult, error) {
	if runtime == nil || runtime.objects == nil {
		return object.DedupeResult{}, errors.New("dedupe runtime unavailable")
	}
	result, err := runtime.objects.RunDedupe(ctx)
	if err != nil {
		return object.DedupeResult{}, fmt.Errorf("run object dedupe: %w", err)
	}
	return result, nil
}

func (runtime *Runtime) Status() Status {
	if runtime == nil {
		return Status{}
	}
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtime.status
}

func (runtime *Runtime) markStarted() {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.status.Running = true
	runtime.status.LastStartedAt = time.Now()
	runtime.status.LastError = ""
}

func (runtime *Runtime) markFinished(result object.DedupeResult, err error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.status.Running = false
	runtime.status.LastFinishedAt = time.Now()
	runtime.status.LastResult = result
	if err != nil {
		runtime.status.LastError = err.Error()
		return
	}
	runtime.status.LastError = ""
}
