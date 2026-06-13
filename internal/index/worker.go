package index

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	defaultIndexWorkerID         = "maxio-index-worker"
	defaultIndexWorkerBatchSize  = 16
	defaultIndexWorkerJobTimeout = 30 * time.Second
)

type JobRepository interface {
	LeaseRunnableIndexJobs(ctx context.Context, request LeaseRequest) ([]Job, error)
	MarkIndexJobSucceeded(ctx context.Context, job Job) error
	MarkIndexJobFailed(ctx context.Context, job Job) error
}

type LeaseRequest struct {
	Now            time.Time
	WorkerID       string
	Limit          int
	RunningTimeout time.Duration
	RetryPolicy    RetryPolicy
}

type JobProcessor interface {
	ProcessIndexJob(ctx context.Context, job Job) error
}

type JobProcessorFunc func(ctx context.Context, job Job) error

func (fn JobProcessorFunc) ProcessIndexJob(ctx context.Context, job Job) error {
	return fn(ctx, job)
}

type WorkerOptions struct {
	WorkerID       string
	BatchSize      int
	JobTimeout     time.Duration
	RunningTimeout time.Duration
	RetryPolicy    RetryPolicy
	Logger         *slog.Logger
	Now            func() time.Time
}

type Worker struct {
	repository JobRepository
	processor  JobProcessor
	options    WorkerOptions
	logger     *slog.Logger
}

type RunResult struct {
	Leased    int
	Succeeded int
	Failed    int
	Retried   int
	Exhausted int
}

func NewWorker(repository JobRepository, processor JobProcessor, options WorkerOptions) *Worker {
	options = options.normalized()
	return &Worker{
		repository: repository,
		processor:  processor,
		options:    options,
		logger:     options.Logger,
	}
}

func (worker *Worker) RunOnce(ctx context.Context) (RunResult, error) {
	if worker == nil || worker.repository == nil || worker.processor == nil {
		return RunResult{}, errors.New("index worker unavailable")
	}
	options := worker.options.normalized()
	now := options.now()
	jobs, err := worker.repository.LeaseRunnableIndexJobs(ctx, LeaseRequest{
		Now:            now,
		WorkerID:       options.WorkerID,
		Limit:          options.BatchSize,
		RunningTimeout: options.RunningTimeout,
		RetryPolicy:    options.RetryPolicy,
	})
	if err != nil {
		return RunResult{}, fmt.Errorf("lease index jobs: %w", err)
	}

	result := RunResult{Leased: len(jobs)}
	for i := range jobs {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("index worker context: %w", err)
		}
		if err := worker.runLeasedJob(ctx, jobs[i], options, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (worker *Worker) runLeasedJob(ctx context.Context, job Job, options WorkerOptions, result *RunResult) error {
	if job.currentState() != JobStateRunning {
		return fmt.Errorf("%w: leased index job %q is %q", ErrInvalidJobTransition, job.ID, job.State)
	}
	processErr := worker.process(ctx, job, options)
	finishedAt := options.now()
	if processErr == nil {
		succeeded, err := job.Succeed(finishedAt)
		if err != nil {
			return err
		}
		if err := worker.repository.MarkIndexJobSucceeded(ctx, succeeded); err != nil {
			return fmt.Errorf("mark index job succeeded: %w", err)
		}
		result.Succeeded++
		return nil
	}

	failed, err := job.Fail(finishedAt, processErr, options.RetryPolicy)
	if err != nil {
		return err
	}
	if err := worker.repository.MarkIndexJobFailed(ctx, failed); err != nil {
		return fmt.Errorf("mark index job failed: %w", err)
	}
	result.Failed++
	if failed.RetryAt == nil {
		result.Exhausted++
		worker.log().WarnContext(ctx, "index job failed permanently", "job_id", failed.ID, "error", failed.LastError)
		return nil
	}
	result.Retried++
	worker.log().DebugContext(ctx, "index job scheduled for retry", "job_id", failed.ID, "retry_at", failed.RetryAt)
	return nil
}

func (worker *Worker) process(ctx context.Context, job Job, options WorkerOptions) error {
	processCtx := ctx
	cancel := func() {}
	if options.JobTimeout > 0 {
		processCtx, cancel = context.WithTimeout(ctx, options.JobTimeout)
	}
	defer cancel()

	err := worker.processor.ProcessIndexJob(processCtx, job)
	if err == nil && processCtx.Err() != nil {
		return fmt.Errorf("index job context: %w", processCtx.Err())
	}
	if err != nil {
		return fmt.Errorf("process index job: %w", err)
	}
	return nil
}

func (worker *Worker) log() *slog.Logger {
	if worker == nil || worker.logger == nil {
		return slog.Default()
	}
	return worker.logger
}

func (options WorkerOptions) normalized() WorkerOptions {
	options.WorkerID = strings.TrimSpace(options.WorkerID)
	if options.WorkerID == "" {
		options.WorkerID = defaultIndexWorkerID
	}
	if options.BatchSize <= 0 {
		options.BatchSize = defaultIndexWorkerBatchSize
	}
	if options.JobTimeout <= 0 {
		options.JobTimeout = defaultIndexWorkerJobTimeout
	}
	if options.RunningTimeout <= 0 {
		options.RunningTimeout = 2 * options.JobTimeout
	}
	options.RetryPolicy = options.RetryPolicy.normalized()
	if options.Logger == nil {
		options.Logger = slog.Default()
	}
	if options.Now == nil {
		options.Now = func() time.Time {
			return time.Now().UTC()
		}
	}
	return options
}

func (options WorkerOptions) now() time.Time {
	return jobTime(options.Now())
}
