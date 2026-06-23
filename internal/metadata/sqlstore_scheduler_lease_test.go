package metadata

import (
	"context"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/internal/scheduler"
)

func TestSQLSchedulerLeaseRepositoryAcquireBlocksSecondOwner(t *testing.T) {
	repository, now := newTestSQLSchedulerLeaseRepository(t)
	ctx := context.Background()
	first := testSchedulerLease("replica-a", now)
	second := testSchedulerLease("replica-b", now)

	acquired := mustAcquireSchedulerLease(ctx, t, repository, first)
	if acquired.Owner != first.Owner {
		t.Fatalf("lease owner = %q, want %q", acquired.Owner, first.Owner)
	}
	blocked := mustBlockSchedulerLease(ctx, t, repository, second)
	if blocked.Owner != first.Owner {
		t.Fatalf("blocking owner = %q, want %q", blocked.Owner, first.Owner)
	}
}

func TestSQLSchedulerLeaseRepositoryHeartbeatAndRelease(t *testing.T) {
	repository, now := newTestSQLSchedulerLeaseRepository(t)
	ctx := context.Background()
	first := testSchedulerLease("replica-a", now)
	second := testSchedulerLease("replica-b", now)

	mustAcquireSchedulerLease(ctx, t, repository, first)
	assertSQLSchedulerHeartbeatLease(ctx, t, repository, first, now.Add(2*time.Minute))
	assertSQLSchedulerWrongOwnerReleaseIgnored(ctx, t, repository, second)
	assertSQLSchedulerOwnerRelease(ctx, t, repository, first)
	mustAcquireSchedulerLease(ctx, t, repository, second)
}

func TestSQLSchedulerLeaseRepositoryAcquireExpiredLease(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	store := newTestSQLiteCatalogStore(t)
	repository := NewSQLSchedulerLeaseRepository(store, WithSQLSchedulerLeaseClock(func() time.Time {
		return now
	}))
	ctx := context.Background()
	first := scheduler.Lease{
		TaskName:  "partitioned",
		Scope:     "partition-1",
		Owner:     "replica-a",
		ExpiresAt: now.Add(time.Minute),
	}
	mustAcquireSchedulerLease(ctx, t, repository, first)

	now = now.Add(time.Minute + time.Nanosecond)
	second := scheduler.Lease{
		TaskName:  first.TaskName,
		Scope:     first.Scope,
		Owner:     "replica-b",
		ExpiresAt: now.Add(time.Minute),
	}
	acquired := mustAcquireSchedulerLease(ctx, t, repository, second)
	if acquired.Owner != second.Owner {
		t.Fatalf("acquired owner = %q, want %q", acquired.Owner, second.Owner)
	}
}

func TestNewSchedulerLeaseRepositoryRequiresSQLMetadata(t *testing.T) {
	repository, err := newSchedulerLeaseRepository(NewInMemoryMetadata())
	if err == nil {
		t.Fatal("expected non-SQL metadata backend to be rejected")
	}
	if repository != nil {
		t.Fatalf("repository = %T, want nil", repository)
	}
}

func newTestSQLSchedulerLeaseRepository(t *testing.T) (*SQLSchedulerLeaseRepository, time.Time) {
	t.Helper()
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	store := newTestSQLiteCatalogStore(t)
	repository := NewSQLSchedulerLeaseRepository(store, WithSQLSchedulerLeaseClock(func() time.Time {
		return now
	}))
	return repository, now
}

func testSchedulerLease(owner string, now time.Time) scheduler.Lease {
	return scheduler.Lease{
		TaskName:  "index",
		Scope:     scheduler.LeaseScopeGlobal,
		Owner:     owner,
		ExpiresAt: now.Add(time.Minute),
	}
}

func mustAcquireSchedulerLease(
	ctx context.Context,
	t *testing.T,
	repository *SQLSchedulerLeaseRepository,
	lease scheduler.Lease,
) scheduler.Lease {
	t.Helper()
	acquired, ok, err := repository.Acquire(ctx, lease)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if !ok {
		t.Fatal("expected lease acquisition")
	}
	return acquired
}

func mustBlockSchedulerLease(
	ctx context.Context,
	t *testing.T,
	repository *SQLSchedulerLeaseRepository,
	lease scheduler.Lease,
) scheduler.Lease {
	t.Helper()
	blocked, ok, err := repository.Acquire(ctx, lease)
	if err != nil {
		t.Fatalf("acquire contended lease: %v", err)
	}
	if ok {
		t.Fatal("expected active lease to block second owner")
	}
	return blocked
}

func assertSQLSchedulerHeartbeatLease(
	ctx context.Context,
	t *testing.T,
	repository *SQLSchedulerLeaseRepository,
	lease scheduler.Lease,
	expiresAt time.Time,
) {
	t.Helper()
	heartbeat := lease
	heartbeat.ExpiresAt = expiresAt
	renewed, ok, err := repository.Heartbeat(ctx, heartbeat)
	if err != nil {
		t.Fatalf("heartbeat lease: %v", err)
	}
	if !ok {
		t.Fatal("expected heartbeat to renew owner lease")
	}
	if !renewed.ExpiresAt.Equal(heartbeat.ExpiresAt) {
		t.Fatalf("renewed expires_at = %s, want %s", renewed.ExpiresAt, heartbeat.ExpiresAt)
	}
}

func assertSQLSchedulerWrongOwnerReleaseIgnored(
	ctx context.Context,
	t *testing.T,
	repository *SQLSchedulerLeaseRepository,
	lease scheduler.Lease,
) {
	t.Helper()
	released, err := repository.Release(ctx, lease)
	if err != nil {
		t.Fatalf("release wrong owner lease: %v", err)
	}
	if released {
		t.Fatal("expected wrong owner release to be ignored")
	}
}

func assertSQLSchedulerOwnerRelease(
	ctx context.Context,
	t *testing.T,
	repository *SQLSchedulerLeaseRepository,
	lease scheduler.Lease,
) {
	t.Helper()
	released, err := repository.Release(ctx, lease)
	if err != nil {
		t.Fatalf("release owner lease: %v", err)
	}
	if !released {
		t.Fatal("expected owner release")
	}
}
