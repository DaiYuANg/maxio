package scheduler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	gocron "github.com/go-co-op/gocron/v2"
)

const (
	// TaskTypeSingleton allows one active lease for a task across all scheduler replicas.
	TaskTypeSingleton TaskType = "singleton"
	// TaskTypePartitioned allows one active lease per task scope across all scheduler replicas.
	TaskTypePartitioned TaskType = "partitioned"
	// TaskTypeParallel records a lease per execution scope without globally serializing the task.
	TaskTypeParallel TaskType = "parallel"

	LeaseScopeGlobal = "global"

	defaultLeaseTTL = 30 * time.Second
)

var (
	ErrInvalidLease               = errors.New("invalid scheduler lease")
	ErrLeaseRepositoryUnavailable = errors.New("scheduler lease repository unavailable")
)

type TaskType string

type Lease struct {
	TaskName  string
	Scope     string
	Owner     string
	ExpiresAt time.Time
}

type LeaseRepository interface {
	Acquire(ctx context.Context, lease Lease) (Lease, bool, error)
	Heartbeat(ctx context.Context, lease Lease) (Lease, bool, error)
	Release(ctx context.Context, lease Lease) (bool, error)
}

type LeaseSpec struct {
	TaskName          string
	TaskType          TaskType
	Scope             string
	TTL               time.Duration
	HeartbeatInterval time.Duration
	Owner             string
}

func (runtime *Runtime) NewLeasedJob(
	definition gocron.JobDefinition,
	lease LeaseSpec,
	run func(context.Context),
	options ...gocron.JobOption,
) (gocron.Job, error) {
	if runtime == nil || runtime.scheduler == nil {
		return nil, errors.New("scheduler unavailable")
	}
	if run == nil {
		return nil, errors.New("scheduled task unavailable")
	}
	lease, err := runtime.prepareLeaseSpec(lease)
	if err != nil {
		return nil, err
	}
	return runtime.NewJob(
		definition,
		gocron.NewTask(func(runCtx context.Context) {
			runtime.runLeasedTask(runCtx, lease, run)
		}),
		options...,
	)
}

func (runtime *Runtime) prepareLeaseSpec(lease LeaseSpec) (LeaseSpec, error) {
	lease.TaskName = strings.TrimSpace(lease.TaskName)
	if lease.TaskName == "" {
		return LeaseSpec{}, fmt.Errorf("%w: task_name is required", ErrInvalidLease)
	}

	taskType, err := normalizeTaskType(lease.TaskType)
	if err != nil {
		return LeaseSpec{}, err
	}
	lease.TaskType = taskType

	scope, err := normalizeLeaseScope(lease.TaskType, lease.Scope)
	if err != nil {
		return LeaseSpec{}, err
	}
	lease.Scope = scope

	lease.TTL, lease.HeartbeatInterval = normalizeLeaseDurations(lease.TTL, lease.HeartbeatInterval)
	lease.Owner = runtime.normalizeLeaseOwner(lease.Owner)
	return lease, nil
}

func (runtime *Runtime) runLeasedTask(ctx context.Context, spec LeaseSpec, run func(context.Context)) {
	if runtime == nil || run == nil {
		return
	}
	repository := runtime.leaseRepository
	if repository == nil {
		runtime.logMissingLeaseRepository(ctx, spec)
		return
	}

	acquired, ok := runtime.acquireTaskLease(ctx, repository, spec)
	if !ok {
		return
	}
	runtime.executeLeasedTask(ctx, repository, acquired, spec, run)
}

func (runtime *Runtime) acquireTaskLease(ctx context.Context, repository LeaseRepository, spec LeaseSpec) (Lease, bool) {
	lease := runtime.leaseForRun(spec)
	acquired, ok, err := repository.Acquire(ctx, lease)
	if err != nil {
		if runtime.logger != nil {
			runtime.logger.ErrorContext(ctx, "acquire scheduler lease failed",
				"task", lease.TaskName,
				"scope", lease.Scope,
				"owner", lease.Owner,
				"error", err,
			)
		}
		return Lease{}, false
	}
	if !ok {
		if runtime.logger != nil {
			runtime.logger.DebugContext(ctx, "skip scheduled task with active lease",
				"task", lease.TaskName,
				"scope", lease.Scope,
				"owner", lease.Owner,
				"lease_owner", acquired.Owner,
				"lease_expires_at", acquired.ExpiresAt,
			)
		}
		return Lease{}, false
	}
	return acquired, true
}

func (runtime *Runtime) executeLeasedTask(
	ctx context.Context,
	repository LeaseRepository,
	acquired Lease,
	spec LeaseSpec,
	run func(context.Context),
) {
	taskCtx, cancel := context.WithCancel(ctx)
	stopHeartbeat := runtime.startLeaseHeartbeat(taskCtx, cancel, repository, acquired, spec)
	defer func() {
		cancel()
		stopHeartbeat()
		runtime.releaseTaskLease(ctx, repository, acquired)
	}()

	run(taskCtx)
}

func (runtime *Runtime) releaseTaskLease(ctx context.Context, repository LeaseRepository, lease Lease) {
	releaseCtx := context.WithoutCancel(ctx)
	released, err := repository.Release(releaseCtx, lease)
	if err != nil {
		if runtime.logger != nil {
			runtime.logger.ErrorContext(ctx, "release scheduler lease failed",
				"task", lease.TaskName,
				"scope", lease.Scope,
				"owner", lease.Owner,
				"error", err,
			)
		}
		return
	}
	if runtime.logger != nil && released {
		runtime.logger.DebugContext(ctx, "released scheduler lease",
			"task", lease.TaskName,
			"scope", lease.Scope,
			"owner", lease.Owner,
		)
	}
}

func (runtime *Runtime) logMissingLeaseRepository(ctx context.Context, spec LeaseSpec) {
	if runtime.logger == nil {
		return
	}
	runtime.logger.ErrorContext(ctx, "skip scheduled task without lease repository",
		"task", spec.TaskName,
		"type", spec.TaskType,
		"scope", spec.Scope,
	)
}

func (runtime *Runtime) leaseForRun(spec LeaseSpec) Lease {
	scope := spec.Scope
	if spec.TaskType == TaskTypeParallel && scope == "" {
		sequence := atomic.AddUint64(&runtime.leaseSequence, 1)
		scope = spec.Owner + "/" + strconv.FormatInt(time.Now().UTC().UnixNano(), 10) + "/" + strconv.FormatUint(sequence, 10)
	}
	return Lease{
		TaskName:  spec.TaskName,
		Scope:     scope,
		Owner:     spec.Owner,
		ExpiresAt: time.Now().UTC().Add(spec.TTL),
	}
}

func normalizeLeaseIdentity(lease Lease) (Lease, error) {
	lease.TaskName = strings.TrimSpace(lease.TaskName)
	lease.Scope = strings.TrimSpace(lease.Scope)
	lease.Owner = strings.TrimSpace(lease.Owner)
	if lease.TaskName == "" {
		return Lease{}, fmt.Errorf("%w: task_name is required", ErrInvalidLease)
	}
	if lease.Scope == "" {
		return Lease{}, fmt.Errorf("%w: scope is required", ErrInvalidLease)
	}
	if lease.Owner == "" {
		return Lease{}, fmt.Errorf("%w: owner is required", ErrInvalidLease)
	}
	return lease, nil
}

func normalizeExpiringLease(lease Lease) (Lease, error) {
	lease, err := normalizeLeaseIdentity(lease)
	if err != nil {
		return Lease{}, err
	}
	if lease.ExpiresAt.IsZero() {
		return Lease{}, fmt.Errorf("%w: expires_at is required", ErrInvalidLease)
	}
	lease.ExpiresAt = lease.ExpiresAt.UTC()
	return lease, nil
}

func defaultLeaseOwner() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	return strings.TrimSpace(host) + ":" + strconv.Itoa(os.Getpid())
}
