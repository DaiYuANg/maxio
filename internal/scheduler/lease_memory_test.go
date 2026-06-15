package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryLeaseRepositoryAcquireBlocksSecondOwner(t *testing.T) {
	repository, now := newTestLeaseRepository()
	ctx := context.Background()
	first := testLease("replica-a", now)
	second := testLease("replica-b", now)

	acquired := mustAcquireLease(ctx, t, repository, first)
	if acquired.Owner != first.Owner {
		t.Fatalf("lease owner = %q, want %q", acquired.Owner, first.Owner)
	}
	blocked := mustBlockLease(ctx, t, repository, second)
	if blocked.Owner != first.Owner {
		t.Fatalf("blocking owner = %q, want %q", blocked.Owner, first.Owner)
	}
}

func TestInMemoryLeaseRepositoryHeartbeatAndRelease(t *testing.T) {
	repository, now := newTestLeaseRepository()
	ctx := context.Background()
	first := testLease("replica-a", now)
	second := testLease("replica-b", now)

	mustAcquireLease(ctx, t, repository, first)
	assertHeartbeatLease(ctx, t, repository, first, now.Add(2*time.Minute))
	assertWrongOwnerReleaseIgnored(ctx, t, repository, second)
	assertOwnerRelease(ctx, t, repository, first)
	mustAcquireLease(ctx, t, repository, second)
}

func TestInMemoryLeaseRepositoryAcquireExpiredLease(t *testing.T) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	repository := NewInMemoryLeaseRepository(WithInMemoryLeaseClock(func() time.Time {
		return now
	}))
	ctx := context.Background()
	first := Lease{
		TaskName:  "partitioned",
		Scope:     "shard-1",
		Owner:     "replica-a",
		ExpiresAt: now.Add(time.Minute),
	}
	mustAcquireLease(ctx, t, repository, first)

	now = now.Add(time.Minute + time.Nanosecond)
	second := Lease{
		TaskName:  first.TaskName,
		Scope:     first.Scope,
		Owner:     "replica-b",
		ExpiresAt: now.Add(time.Minute),
	}
	acquired := mustAcquireLease(ctx, t, repository, second)
	if acquired.Owner != second.Owner {
		t.Fatalf("acquired owner = %q, want %q", acquired.Owner, second.Owner)
	}
}

func newTestLeaseRepository() (*InMemoryLeaseRepository, time.Time) {
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	repository := NewInMemoryLeaseRepository(WithInMemoryLeaseClock(func() time.Time {
		return now
	}))
	return repository, now
}

func testLease(owner string, now time.Time) Lease {
	return Lease{
		TaskName:  "index",
		Scope:     LeaseScopeGlobal,
		Owner:     owner,
		ExpiresAt: now.Add(time.Minute),
	}
}

func mustAcquireLease(ctx context.Context, t *testing.T, repository *InMemoryLeaseRepository, lease Lease) Lease {
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

func mustBlockLease(ctx context.Context, t *testing.T, repository *InMemoryLeaseRepository, lease Lease) Lease {
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

func assertHeartbeatLease(
	ctx context.Context,
	t *testing.T,
	repository *InMemoryLeaseRepository,
	lease Lease,
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

func assertWrongOwnerReleaseIgnored(ctx context.Context, t *testing.T, repository *InMemoryLeaseRepository, lease Lease) {
	t.Helper()
	released, err := repository.Release(ctx, lease)
	if err != nil {
		t.Fatalf("release wrong owner lease: %v", err)
	}
	if released {
		t.Fatal("expected wrong owner release to be ignored")
	}
}

func assertOwnerRelease(ctx context.Context, t *testing.T, repository *InMemoryLeaseRepository, lease Lease) {
	t.Helper()
	released, err := repository.Release(ctx, lease)
	if err != nil {
		t.Fatalf("release owner lease: %v", err)
	}
	if !released {
		t.Fatal("expected owner release")
	}
}
