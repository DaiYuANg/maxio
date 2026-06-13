package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type InMemoryLeaseRepository struct {
	mu     sync.Mutex
	leases map[leaseKey]Lease
	now    func() time.Time
}

type InMemoryLeaseRepositoryOption func(*InMemoryLeaseRepository)

func WithInMemoryLeaseClock(now func() time.Time) InMemoryLeaseRepositoryOption {
	return func(repository *InMemoryLeaseRepository) {
		if now != nil {
			repository.now = now
		}
	}
}

func NewInMemoryLeaseRepository(options ...InMemoryLeaseRepositoryOption) *InMemoryLeaseRepository {
	repository := &InMemoryLeaseRepository{
		leases: make(map[leaseKey]Lease),
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

func (repository *InMemoryLeaseRepository) Acquire(ctx context.Context, lease Lease) (Lease, bool, error) {
	if repository == nil {
		return Lease{}, false, ErrLeaseRepositoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return Lease{}, false, fmt.Errorf("lease acquire context: %w", err)
	}
	lease, err := repository.normalizeFutureLease(lease)
	if err != nil {
		return Lease{}, false, err
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()

	key := newLeaseKey(lease)
	existing, ok := repository.leases[key]
	if ok && repository.isActive(existing) {
		return existing, false, nil
	}
	repository.leases[key] = lease
	return lease, true, nil
}

func (repository *InMemoryLeaseRepository) Heartbeat(ctx context.Context, lease Lease) (Lease, bool, error) {
	if repository == nil {
		return Lease{}, false, ErrLeaseRepositoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return Lease{}, false, fmt.Errorf("lease heartbeat context: %w", err)
	}
	lease, err := repository.normalizeFutureLease(lease)
	if err != nil {
		return Lease{}, false, err
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()

	key := newLeaseKey(lease)
	existing, ok := repository.leases[key]
	if !ok {
		return Lease{}, false, nil
	}
	if !repository.isActive(existing) {
		delete(repository.leases, key)
		return Lease{}, false, nil
	}
	if existing.Owner != lease.Owner {
		return existing, false, nil
	}

	repository.leases[key] = lease
	return lease, true, nil
}

func (repository *InMemoryLeaseRepository) Release(ctx context.Context, lease Lease) (bool, error) {
	if repository == nil {
		return false, ErrLeaseRepositoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("lease release context: %w", err)
	}
	lease, err := normalizeLeaseIdentity(lease)
	if err != nil {
		return false, err
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()

	key := newLeaseKey(lease)
	existing, ok := repository.leases[key]
	if !ok {
		return false, nil
	}
	if !repository.isActive(existing) {
		delete(repository.leases, key)
		return false, nil
	}
	if existing.Owner != lease.Owner {
		return false, nil
	}

	delete(repository.leases, key)
	return true, nil
}

func (repository *InMemoryLeaseRepository) normalizeFutureLease(lease Lease) (Lease, error) {
	lease, err := normalizeExpiringLease(lease)
	if err != nil {
		return Lease{}, err
	}
	if !repository.now().Before(lease.ExpiresAt) {
		return Lease{}, fmt.Errorf("%w: expires_at must be in the future", ErrInvalidLease)
	}
	return lease, nil
}

func (repository *InMemoryLeaseRepository) isActive(lease Lease) bool {
	return repository.now().Before(lease.ExpiresAt)
}

type leaseKey struct {
	taskName string
	scope    string
}

func newLeaseKey(lease Lease) leaseKey {
	return leaseKey{
		taskName: lease.TaskName,
		scope:    lease.Scope,
	}
}
