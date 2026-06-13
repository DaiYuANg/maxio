package index

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIndexJobRetryStateTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	policy := testRetryPolicy(2, time.Second)
	job := NewJob("job-1", JobOperationUpsert, "docs", "guide.txt", now)
	assertRunnable(t, job, now, policy)

	running := mustStartJob(t, job, now, policy)
	assertRunningJob(t, running, 1)

	failedAt := now.Add(100 * time.Millisecond)
	failed := mustFailJob(t, running, failedAt, errors.New("temporary failure"), policy)
	assertFailedRetry(t, failed, failedAt.Add(time.Second), policy)

	retry := mustStartJob(t, failed, failedAt.Add(time.Second), policy)
	succeeded := mustSucceedJob(t, retry, failedAt.Add(2*time.Second))
	assertSucceededJob(t, succeeded)
}

func TestIndexJobFinalFailureHasNoRetryAt(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC)
	policy := testRetryPolicy(1, time.Second)
	running := mustStartJob(t, NewJob("job-2", JobOperationDelete, "docs", "old.txt", now), now, policy)
	failed := mustFailJob(t, running, now.Add(time.Second), errors.New("permanent failure"), policy)
	assertFinalFailure(t, failed, now, policy)
}

func TestIndexJobTimeoutBoundaryAndReclaim(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	timeout := 30 * time.Second
	policy := testRetryPolicy(2, 2*time.Second)
	running := mustStartJob(t, NewJob("job-3", JobOperationUpsert, "docs", "slow.txt", now), now, policy)

	assertTimeoutBoundary(t, running, now, timeout, policy)
	reclaimed := mustReclaimJob(t, running, now.Add(timeout), timeout, policy)
	assertRunningJob(t, reclaimed, 2)

	timedOut := mustTimeoutJob(t, reclaimed, now.Add(2*timeout), timeout, policy)
	assertFinalFailure(t, timedOut, now, policy)
}

func TestRetryBackoffCapsAtMaxBackoff(t *testing.T) {
	t.Parallel()

	policy := RetryPolicy{
		MaxAttempts:    5,
		InitialBackoff: time.Second,
		MaxBackoff:     3 * time.Second,
	}
	if got := policy.Backoff(4); got != 3*time.Second {
		t.Fatalf("backoff = %s, want 3s", got)
	}
}

func TestWorkerMarksFailedJobForRetry(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 6, 13, 13, 0, 0, 0, time.UTC)
	policy := testRetryPolicy(2, time.Second)
	running := mustStartJob(t, NewJob("job-4", JobOperationUpsert, "docs", "retry.txt", now), now, policy)
	repository := &fakeIndexJobRepository{leased: []Job{running}}
	worker := newFailingIndexWorker(repository, policy, now)

	result, err := worker.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("run worker: %v", err)
	}
	assertFailedWorkerResult(t, result, repository)
}

func testRetryPolicy(maxAttempts int, backoff time.Duration) RetryPolicy {
	return RetryPolicy{
		MaxAttempts:    maxAttempts,
		InitialBackoff: backoff,
		MaxBackoff:     backoff,
	}
}

func assertRunnable(t *testing.T, job Job, now time.Time, policy RetryPolicy) {
	t.Helper()
	if !job.Runnable(now, policy, time.Minute) {
		t.Fatal("pending job should be runnable immediately")
	}
}

func mustStartJob(t *testing.T, job Job, now time.Time, policy RetryPolicy) Job {
	t.Helper()
	running, err := job.Start(now, policy)
	if err != nil {
		t.Fatalf("start job: %v", err)
	}
	return running
}

func mustFailJob(t *testing.T, job Job, now time.Time, cause error, policy RetryPolicy) Job {
	t.Helper()
	failed, err := job.Fail(now, cause, policy)
	if err != nil {
		t.Fatalf("fail job: %v", err)
	}
	return failed
}

func mustSucceedJob(t *testing.T, job Job, now time.Time) Job {
	t.Helper()
	succeeded, err := job.Succeed(now)
	if err != nil {
		t.Fatalf("succeed job: %v", err)
	}
	return succeeded
}

func mustReclaimJob(t *testing.T, job Job, now time.Time, timeout time.Duration, policy RetryPolicy) Job {
	t.Helper()
	reclaimed, err := job.ReclaimTimedOut(now, timeout, policy)
	if err != nil {
		t.Fatalf("reclaim timed-out job: %v", err)
	}
	return reclaimed
}

func mustTimeoutJob(t *testing.T, job Job, now time.Time, timeout time.Duration, policy RetryPolicy) Job {
	t.Helper()
	timedOut, err := job.Timeout(now, timeout, policy)
	if err != nil {
		t.Fatalf("mark timed-out job: %v", err)
	}
	return timedOut
}

func assertRunningJob(t *testing.T, job Job, attempts int) {
	t.Helper()
	if job.State != JobStateRunning || job.Attempts != attempts || job.RetryAt != nil {
		t.Fatalf("running job = %+v", job)
	}
}

func assertFailedRetry(t *testing.T, job Job, retryAt time.Time, policy RetryPolicy) {
	t.Helper()
	if job.State != JobStateFailed || job.RetryAt == nil {
		t.Fatalf("failed job = %+v", job)
	}
	if !job.RetryAt.Equal(retryAt) {
		t.Fatalf("retry_at = %s, want %s", job.RetryAt, retryAt)
	}
	if job.Runnable(retryAt.Add(-time.Nanosecond), policy, time.Minute) {
		t.Fatal("failed job should not run before retry_at")
	}
	if !job.Runnable(retryAt, policy, time.Minute) {
		t.Fatal("failed job should run at retry_at boundary")
	}
}

func assertSucceededJob(t *testing.T, job Job) {
	t.Helper()
	if job.State != JobStateSucceeded || job.RetryAt != nil || job.LastError != "" {
		t.Fatalf("succeeded job = %+v", job)
	}
}

func assertFinalFailure(t *testing.T, job Job, now time.Time, policy RetryPolicy) {
	t.Helper()
	if job.State != JobStateFailed || job.RetryAt != nil {
		t.Fatalf("final failed job = %+v", job)
	}
	if job.Runnable(now.Add(time.Hour), policy, time.Minute) {
		t.Fatal("final failed job should not be runnable")
	}
}

func assertTimeoutBoundary(t *testing.T, job Job, now time.Time, timeout time.Duration, policy RetryPolicy) {
	t.Helper()
	if job.TimedOut(now.Add(timeout-time.Nanosecond), timeout) {
		t.Fatal("job should not time out before boundary")
	}
	if !job.TimedOut(now.Add(timeout), timeout) {
		t.Fatal("job should time out at boundary")
	}
	if !job.Runnable(now.Add(timeout), policy, timeout) {
		t.Fatal("timed-out running job should be reclaimable when attempts remain")
	}
}

func newFailingIndexWorker(repository *fakeIndexJobRepository, policy RetryPolicy, now time.Time) *Worker {
	return NewWorker(repository, JobProcessorFunc(func(context.Context, Job) error {
		return errors.New("index unavailable")
	}), WorkerOptions{
		RetryPolicy: policy,
		Now: func() time.Time {
			return now
		},
	})
}

func assertFailedWorkerResult(t *testing.T, result RunResult, repository *fakeIndexJobRepository) {
	t.Helper()
	if result.Leased != 1 || result.Failed != 1 || result.Retried != 1 || result.Exhausted != 0 {
		t.Fatalf("run result = %+v", result)
	}
	if len(repository.failed) != 1 {
		t.Fatalf("failed updates = %d, want 1", len(repository.failed))
	}
	failed := repository.failed[0]
	if failed.State != JobStateFailed || failed.RetryAt == nil || !strings.Contains(failed.LastError, "index unavailable") {
		t.Fatalf("failed update = %+v", failed)
	}
}

type fakeIndexJobRepository struct {
	leased    []Job
	succeeded []Job
	failed    []Job
}

func (repository *fakeIndexJobRepository) LeaseRunnableIndexJobs(context.Context, LeaseRequest) ([]Job, error) {
	return repository.leased, nil
}

func (repository *fakeIndexJobRepository) MarkIndexJobSucceeded(_ context.Context, job Job) error {
	repository.succeeded = append(repository.succeeded, job)
	return nil
}

func (repository *fakeIndexJobRepository) MarkIndexJobFailed(_ context.Context, job Job) error {
	repository.failed = append(repository.failed, job)
	return nil
}
