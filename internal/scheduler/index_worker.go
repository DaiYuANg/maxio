package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	gocron "github.com/go-co-op/gocron/v2"
	searchindex "github.com/lyonbrown4d/maxio/internal/index"
)

const IndexWorkerJobName = "maxio.index.worker"
const defaultIndexWorkerInterval = 5 * time.Second

func (runtime *Runtime) ScheduleIndexWorker(
	ctx context.Context,
	worker *searchindex.Worker,
	interval time.Duration,
	options ...gocron.JobOption,
) (gocron.Job, error) {
	if runtime == nil {
		return nil, errors.New("scheduler unavailable")
	}
	if worker == nil {
		return nil, errors.New("index worker unavailable")
	}
	if interval <= 0 {
		interval = defaultIndexWorkerInterval
	}
	jobOptions := append(
		[]gocron.JobOption{
			gocron.WithName(IndexWorkerJobName),
			gocron.WithTags("index", "outbox"),
		},
		options...,
	)
	job, err := runtime.NewLeasedJob(
		gocron.DurationJob(interval),
		LeaseSpec{
			TaskName: IndexWorkerJobName,
			TaskType: TaskTypeSingleton,
			Scope:    LeaseScopeGlobal,
		},
		func(runCtx context.Context) {
			runtime.runIndexWorker(runCtx, worker)
		},
		jobOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("schedule index worker: %w", err)
	}
	if runtime.logger != nil {
		runtime.logger.InfoContext(ctx, "index worker scheduled",
			"job", IndexWorkerJobName,
			"interval", interval.String(),
		)
	}
	return job, nil
}

func (runtime *Runtime) runIndexWorker(ctx context.Context, worker *searchindex.Worker) {
	if runtime == nil || worker == nil {
		return
	}
	result, err := worker.RunOnce(ctx)
	if err != nil {
		if runtime.logger != nil {
			runtime.logger.ErrorContext(ctx, "index worker failed", "error", err)
		}
		return
	}
	if runtime.logger != nil && result.Leased > 0 {
		runtime.logger.DebugContext(ctx, "index worker completed",
			"leased", result.Leased,
			"succeeded", result.Succeeded,
			"failed", result.Failed,
			"retried", result.Retried,
			"exhausted", result.Exhausted,
		)
	}
}
