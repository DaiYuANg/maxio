package index

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultIndexJobMaxAttempts    = 3
	defaultIndexJobInitialBackoff = time.Second
	defaultIndexJobMaxBackoff     = time.Minute
	maxIndexJobErrorLength        = 2048
)

var (
	ErrInvalidJobTransition      = errors.New("invalid index job transition")
	ErrIndexJobAttemptsExhausted = errors.New("index job attempts exhausted")
	ErrIndexJobRetryNotDue       = errors.New("index job retry is not due")
	ErrIndexJobTimedOut          = errors.New("index job timed out")
)

type JobState string

const (
	JobStatePending   JobState = "pending"
	JobStateRunning   JobState = "running"
	JobStateSucceeded JobState = "succeeded"
	JobStateFailed    JobState = "failed"
)

func (state JobState) Valid() bool {
	switch state {
	case JobStatePending, JobStateRunning, JobStateSucceeded, JobStateFailed:
		return true
	default:
		return false
	}
}

type JobOperation string

const (
	JobOperationUpsert JobOperation = "upsert"
	JobOperationDelete JobOperation = "delete"
)

type Job struct {
	ID          string            `json:"id"`
	Operation   JobOperation      `json:"operation"`
	ObjectID    string            `json:"object_id,omitempty"`
	Bucket      string            `json:"bucket"`
	Key         string            `json:"key"`
	Version     string            `json:"version,omitempty"`
	ETag        string            `json:"etag,omitempty"`
	ContentHash string            `json:"content_hash,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	State       JobState          `json:"state"`
	Attempts    int               `json:"attempts"`
	MaxAttempts int               `json:"max_attempts,omitempty"`
	RetryAt     *time.Time        `json:"retry_at,omitempty"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	FinishedAt  *time.Time        `json:"finished_at,omitempty"`
	LastError   string            `json:"last_error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

func NewJob(id string, operation JobOperation, bucket, key string, now time.Time) Job {
	at := jobTime(now)
	return Job{
		ID:        strings.TrimSpace(id),
		Operation: normalizeJobOperation(operation),
		Bucket:    bucket,
		Key:       key,
		State:     JobStatePending,
		CreatedAt: at,
		UpdatedAt: at,
	}
}

func normalizeJobOperation(operation JobOperation) JobOperation {
	operation = JobOperation(strings.TrimSpace(string(operation)))
	if operation == "" {
		return JobOperationUpsert
	}
	return operation
}

type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func (policy RetryPolicy) Backoff(attempt int) time.Duration {
	policy = policy.normalized()
	if attempt < 1 {
		attempt = 1
	}
	delay := policy.InitialBackoff
	for i := 1; i < attempt; i++ {
		if delay >= policy.MaxBackoff/2 {
			return policy.MaxBackoff
		}
		delay *= 2
		if delay >= policy.MaxBackoff {
			return policy.MaxBackoff
		}
	}
	return delay
}

func (policy RetryPolicy) normalized() RetryPolicy {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaultIndexJobMaxAttempts
	}
	if policy.InitialBackoff <= 0 {
		policy.InitialBackoff = defaultIndexJobInitialBackoff
	}
	if policy.MaxBackoff <= 0 {
		policy.MaxBackoff = defaultIndexJobMaxBackoff
	}
	if policy.MaxBackoff < policy.InitialBackoff {
		policy.MaxBackoff = policy.InitialBackoff
	}
	return policy
}

func (job Job) Runnable(now time.Time, policy RetryPolicy, runningTimeout time.Duration) bool {
	switch job.currentState() {
	case JobStatePending:
		return job.retryDue(now) && job.hasAttemptsRemaining(policy)
	case JobStateFailed:
		return job.RetryAt != nil && job.retryDue(now) && job.hasAttemptsRemaining(policy)
	case JobStateRunning:
		return job.TimedOut(now, runningTimeout) && job.hasAttemptsRemaining(policy)
	case JobStateSucceeded:
		return false
	default:
		return false
	}
}

func (job Job) Start(now time.Time, policy RetryPolicy) (Job, error) {
	switch job.currentState() {
	case JobStatePending:
	case JobStateFailed:
		if job.RetryAt == nil {
			return job, fmt.Errorf("%w: failed job has no retry_at", ErrIndexJobAttemptsExhausted)
		}
	case JobStateRunning, JobStateSucceeded:
		return job, fmt.Errorf("%w: cannot start %q job", ErrInvalidJobTransition, job.State)
	default:
		return job, fmt.Errorf("%w: cannot start %q job", ErrInvalidJobTransition, job.State)
	}
	if !job.retryDue(now) {
		return job, ErrIndexJobRetryNotDue
	}
	if !job.hasAttemptsRemaining(policy) {
		return job, ErrIndexJobAttemptsExhausted
	}
	return job.startRunning(now), nil
}

func (job Job) ReclaimTimedOut(now time.Time, runningTimeout time.Duration, policy RetryPolicy) (Job, error) {
	if job.currentState() != JobStateRunning {
		return job, fmt.Errorf("%w: cannot reclaim %q job", ErrInvalidJobTransition, job.State)
	}
	if !job.TimedOut(now, runningTimeout) {
		return job, ErrIndexJobRetryNotDue
	}
	if !job.hasAttemptsRemaining(policy) {
		return job, ErrIndexJobAttemptsExhausted
	}
	return job.startRunning(now), nil
}

func (job Job) Succeed(now time.Time) (Job, error) {
	if job.currentState() != JobStateRunning {
		return job, fmt.Errorf("%w: cannot succeed %q job", ErrInvalidJobTransition, job.State)
	}
	at := jobTime(now)
	next := job
	next.State = JobStateSucceeded
	next.RetryAt = nil
	next.FinishedAt = &at
	next.LastError = ""
	next.UpdatedAt = at
	return next, nil
}

func (job Job) Fail(now time.Time, cause error, policy RetryPolicy) (Job, error) {
	if job.currentState() != JobStateRunning {
		return job, fmt.Errorf("%w: cannot fail %q job", ErrInvalidJobTransition, job.State)
	}
	at := jobTime(now)
	next := job
	next.State = JobStateFailed
	next.FinishedAt = &at
	next.LastError = failureMessage(cause)
	next.UpdatedAt = at
	next.RetryAt = nil
	if next.hasRetryAfterFailure(policy) {
		retryAt := at.Add(policy.Backoff(next.Attempts))
		next.RetryAt = &retryAt
	}
	return next, nil
}

func (job Job) Timeout(now time.Time, runningTimeout time.Duration, policy RetryPolicy) (Job, error) {
	if !job.TimedOut(now, runningTimeout) {
		return job, ErrIndexJobRetryNotDue
	}
	return job.Fail(now, ErrIndexJobTimedOut, policy)
}

func (job Job) TimedOut(now time.Time, runningTimeout time.Duration) bool {
	if job.currentState() != JobStateRunning || runningTimeout <= 0 || job.StartedAt == nil {
		return false
	}
	return !jobTime(now).Before(job.StartedAt.Add(runningTimeout))
}

func (job Job) currentState() JobState {
	if job.State == "" {
		return JobStatePending
	}
	return job.State
}

func (job Job) retryDue(now time.Time) bool {
	return job.RetryAt == nil || !job.RetryAt.After(jobTime(now))
}

func (job Job) hasAttemptsRemaining(policy RetryPolicy) bool {
	return job.Attempts < job.effectiveMaxAttempts(policy)
}

func (job Job) hasRetryAfterFailure(policy RetryPolicy) bool {
	return job.Attempts < job.effectiveMaxAttempts(policy)
}

func (job Job) effectiveMaxAttempts(policy RetryPolicy) int {
	if job.MaxAttempts > 0 {
		return job.MaxAttempts
	}
	return policy.normalized().MaxAttempts
}

func (job Job) startRunning(now time.Time) Job {
	at := jobTime(now)
	next := job
	next.State = JobStateRunning
	next.Attempts++
	next.RetryAt = nil
	next.StartedAt = &at
	next.FinishedAt = nil
	next.LastError = ""
	next.UpdatedAt = at
	return next
}

func failureMessage(cause error) string {
	if cause == nil {
		return ""
	}
	message := strings.TrimSpace(cause.Error())
	if len(message) > maxIndexJobErrorLength {
		return message[:maxIndexJobErrorLength]
	}
	return message
}

func jobTime(value time.Time) time.Time {
	return value.UTC()
}
