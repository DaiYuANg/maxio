package metadata

import (
	"fmt"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/scheduler"
)

func normalizeSchedulerFutureLease(lease scheduler.Lease, now int64) (scheduler.Lease, error) {
	lease, err := normalizeSchedulerExpiringLease(lease)
	if err != nil {
		return scheduler.Lease{}, err
	}
	if !time.Unix(0, now).UTC().Before(lease.ExpiresAt) {
		return scheduler.Lease{}, fmt.Errorf("%w: expires_at must be in the future", scheduler.ErrInvalidLease)
	}
	return lease, nil
}

func (repository *SQLSchedulerLeaseRepository) isActive(lease scheduler.Lease, now int64) bool {
	return now < lease.ExpiresAt.UTC().UnixNano()
}

func normalizeSchedulerExpiringLease(lease scheduler.Lease) (scheduler.Lease, error) {
	lease, err := normalizeSchedulerLeaseIdentity(lease)
	if err != nil {
		return scheduler.Lease{}, err
	}
	if lease.ExpiresAt.IsZero() {
		return scheduler.Lease{}, fmt.Errorf("%w: expires_at is required", scheduler.ErrInvalidLease)
	}
	lease.ExpiresAt = lease.ExpiresAt.UTC()
	return lease, nil
}

func normalizeSchedulerLeaseIdentity(lease scheduler.Lease) (scheduler.Lease, error) {
	lease.TaskName = strings.TrimSpace(lease.TaskName)
	lease.Scope = strings.TrimSpace(lease.Scope)
	lease.Owner = strings.TrimSpace(lease.Owner)
	if lease.TaskName == "" {
		return scheduler.Lease{}, fmt.Errorf("%w: task_name is required", scheduler.ErrInvalidLease)
	}
	if lease.Scope == "" {
		return scheduler.Lease{}, fmt.Errorf("%w: scope is required", scheduler.ErrInvalidLease)
	}
	if lease.Owner == "" {
		return scheduler.Lease{}, fmt.Errorf("%w: owner is required", scheduler.ErrInvalidLease)
	}
	return lease, nil
}

func metadataSchedulerLeaseFromLease(lease scheduler.Lease, now int64) metadataSchedulerLease {
	return metadataSchedulerLease{
		TaskName:  lease.TaskName,
		Scope:     lease.Scope,
		Owner:     lease.Owner,
		ExpiresAt: lease.ExpiresAt.UTC().UnixNano(),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (row metadataSchedulerLease) schedulerLease() scheduler.Lease {
	return scheduler.Lease{
		TaskName:  row.TaskName,
		Scope:     row.Scope,
		Owner:     row.Owner,
		ExpiresAt: time.Unix(0, row.ExpiresAt).UTC(),
	}
}
