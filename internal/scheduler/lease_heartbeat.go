package scheduler

import (
	"context"
	"time"
)

func (runtime *Runtime) startLeaseHeartbeat(
	ctx context.Context,
	cancel context.CancelFunc,
	repository LeaseRepository,
	lease Lease,
	spec LeaseSpec,
) func() {
	interval := spec.HeartbeatInterval
	if interval <= 0 {
		return func() {}
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go runtime.runLeaseHeartbeat(ctx, cancel, repository, heartbeatState{
		current:  lease,
		spec:     spec,
		interval: interval,
		stop:     stop,
		done:     done,
	})

	return func() {
		close(stop)
		<-done
	}
}

type heartbeatState struct {
	current  Lease
	spec     LeaseSpec
	interval time.Duration
	stop     <-chan struct{}
	done     chan<- struct{}
}

func (runtime *Runtime) runLeaseHeartbeat(
	ctx context.Context,
	cancel context.CancelFunc,
	repository LeaseRepository,
	state heartbeatState,
) {
	defer close(state.done)
	ticker := time.NewTicker(state.interval)
	defer ticker.Stop()

	current := state.current
	for {
		select {
		case <-ctx.Done():
			return
		case <-state.stop:
			return
		case <-ticker.C:
			renewed, ok := runtime.heartbeatLease(ctx, repository, current, state.spec)
			if !ok {
				cancel()
				return
			}
			current = renewed
		}
	}
}

func (runtime *Runtime) heartbeatLease(
	ctx context.Context,
	repository LeaseRepository,
	current Lease,
	spec LeaseSpec,
) (Lease, bool) {
	next := current
	next.ExpiresAt = time.Now().UTC().Add(spec.TTL)
	renewed, ok, err := repository.Heartbeat(ctx, next)
	if err != nil {
		runtime.logLeaseHeartbeatError(ctx, next, err)
		return Lease{}, false
	}
	if !ok {
		runtime.logLeaseLost(ctx, next)
		return Lease{}, false
	}
	return renewed, true
}

func (runtime *Runtime) logLeaseHeartbeatError(ctx context.Context, lease Lease, err error) {
	if runtime.logger == nil {
		return
	}
	runtime.logger.ErrorContext(ctx, "heartbeat scheduler lease failed",
		"task", lease.TaskName,
		"scope", lease.Scope,
		"owner", lease.Owner,
		"error", err,
	)
}

func (runtime *Runtime) logLeaseLost(ctx context.Context, lease Lease) {
	if runtime.logger == nil {
		return
	}
	runtime.logger.WarnContext(ctx, "lost scheduler lease",
		"task", lease.TaskName,
		"scope", lease.Scope,
		"owner", lease.Owner,
	)
}
