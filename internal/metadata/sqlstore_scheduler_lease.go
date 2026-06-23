package metadata

import (
	"context"
	"fmt"
	"time"

	"github.com/lyonbrown4d/maxio/internal/scheduler"
)

type SQLSchedulerLeaseRepository struct {
	store *SQLMetadata
	now   func() time.Time
}

type SQLSchedulerLeaseRepositoryOption func(*SQLSchedulerLeaseRepository)

func WithSQLSchedulerLeaseClock(now func() time.Time) SQLSchedulerLeaseRepositoryOption {
	return func(repository *SQLSchedulerLeaseRepository) {
		if now != nil {
			repository.now = now
		}
	}
}

func NewSQLSchedulerLeaseRepository(
	store *SQLMetadata,
	options ...SQLSchedulerLeaseRepositoryOption,
) *SQLSchedulerLeaseRepository {
	if store != nil && store.dbxDB != nil && store.repos.schedulerLeases == nil {
		store.repos = newMetadataSQLRepositories(store.dbxDB)
	}
	repository := &SQLSchedulerLeaseRepository{
		store: store,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, option := range options {
		if option != nil {
			option(repository)
		}
	}
	return repository
}

func (repository *SQLSchedulerLeaseRepository) Acquire(
	ctx context.Context,
	lease scheduler.Lease,
) (scheduler.Lease, bool, error) {
	ctx, lease, now, err := repository.prepareFutureLease(ctx, lease, "acquire")
	if err != nil {
		return scheduler.Lease{}, false, err
	}
	return repository.acquirePreparedLease(ctx, lease, now)
}

func (repository *SQLSchedulerLeaseRepository) Heartbeat(
	ctx context.Context,
	lease scheduler.Lease,
) (scheduler.Lease, bool, error) {
	ctx, lease, now, err := repository.prepareFutureLease(ctx, lease, "heartbeat")
	if err != nil {
		return scheduler.Lease{}, false, err
	}
	renewed, err := repository.renewOwnerLease(ctx, lease, now)
	if err != nil {
		return scheduler.Lease{}, false, err
	}
	if renewed {
		return lease, true, nil
	}
	return repository.currentHeartbeatBlocker(ctx, lease, now)
}

func (repository *SQLSchedulerLeaseRepository) Release(ctx context.Context, lease scheduler.Lease) (bool, error) {
	ctx, lease, now, err := repository.prepareIdentityLease(ctx, lease, "release")
	if err != nil {
		return false, err
	}
	return repository.releasePreparedLease(ctx, lease, now)
}

func (repository *SQLSchedulerLeaseRepository) acquirePreparedLease(
	ctx context.Context,
	lease scheduler.Lease,
	now int64,
) (scheduler.Lease, bool, error) {
	for range 2 {
		acquired, ok, done, err := repository.tryAcquireLease(ctx, lease, now)
		if err != nil || done {
			return acquired, ok, err
		}
	}
	return scheduler.Lease{}, false, nil
}

func (repository *SQLSchedulerLeaseRepository) tryAcquireLease(
	ctx context.Context,
	lease scheduler.Lease,
	now int64,
) (scheduler.Lease, bool, bool, error) {
	inserted, err := repository.insertLease(ctx, lease, now)
	if err != nil {
		return scheduler.Lease{}, false, true, err
	}
	if inserted {
		return lease, true, true, nil
	}

	updated, err := repository.updateExpiredLease(ctx, lease, now)
	if err != nil {
		return scheduler.Lease{}, false, true, err
	}
	if updated {
		return lease, true, true, nil
	}

	existing, found, err := repository.getLease(ctx, lease.TaskName, lease.Scope)
	if err != nil {
		return scheduler.Lease{}, false, true, err
	}
	if found {
		return existing, false, true, nil
	}
	return scheduler.Lease{}, false, false, nil
}

func (repository *SQLSchedulerLeaseRepository) currentHeartbeatBlocker(
	ctx context.Context,
	lease scheduler.Lease,
	now int64,
) (scheduler.Lease, bool, error) {
	existing, found, err := repository.getLease(ctx, lease.TaskName, lease.Scope)
	if err != nil || !found {
		return scheduler.Lease{}, false, err
	}
	if repository.isActive(existing, now) {
		return existing, false, nil
	}
	return scheduler.Lease{}, false, repository.deleteExpiredLease(ctx, lease.TaskName, lease.Scope, now)
}

func (repository *SQLSchedulerLeaseRepository) releasePreparedLease(
	ctx context.Context,
	lease scheduler.Lease,
	now int64,
) (bool, error) {
	existing, found, err := repository.getLease(ctx, lease.TaskName, lease.Scope)
	if err != nil || !found {
		return false, err
	}
	if !repository.isActive(existing, now) {
		return false, repository.deleteExpiredLease(ctx, lease.TaskName, lease.Scope, now)
	}
	if existing.Owner != lease.Owner {
		return false, nil
	}
	return repository.deleteOwnerLease(ctx, lease, now)
}

func (repository *SQLSchedulerLeaseRepository) prepareFutureLease(
	ctx context.Context,
	lease scheduler.Lease,
	op string,
) (context.Context, scheduler.Lease, int64, error) {
	ctx, now, err := repository.prepareLeaseOperation(ctx, op)
	if err != nil {
		return nil, scheduler.Lease{}, 0, err
	}
	lease, err = normalizeSchedulerFutureLease(lease, now)
	return ctx, lease, now, err
}

func (repository *SQLSchedulerLeaseRepository) prepareIdentityLease(
	ctx context.Context,
	lease scheduler.Lease,
	op string,
) (context.Context, scheduler.Lease, int64, error) {
	ctx, now, err := repository.prepareLeaseOperation(ctx, op)
	if err != nil {
		return nil, scheduler.Lease{}, 0, err
	}
	lease, err = normalizeSchedulerLeaseIdentity(lease)
	return ctx, lease, now, err
}

func (repository *SQLSchedulerLeaseRepository) prepareLeaseOperation(
	ctx context.Context,
	op string,
) (context.Context, int64, error) {
	if repository == nil || repository.store == nil || repository.store.dbxDB == nil {
		return nil, 0, scheduler.ErrLeaseRepositoryUnavailable
	}
	ctx = ensureContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, 0, fmt.Errorf("lease %s context: %w", op, err)
	}
	return ctx, repository.now().UTC().UnixNano(), nil
}
