package metadata

import (
	"context"
	"fmt"

	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/querydsl"
	repositoryx "github.com/arcgolabs/dbx/repository"
	"github.com/lyonbrown4d/maxio/internal/scheduler"
)

func (repository *SQLSchedulerLeaseRepository) insertLease(
	ctx context.Context,
	lease scheduler.Lease,
	now int64,
) (bool, error) {
	row := metadataSchedulerLeaseFromLease(lease, now)
	query := querydsl.InsertInto(metadataSchedulerLeases.schema).
		Values(
			metadataSchedulerLeases.taskName.Set(row.TaskName),
			metadataSchedulerLeases.scope.Set(row.Scope),
			metadataSchedulerLeases.owner.Set(row.Owner),
			metadataSchedulerLeases.expiresAt.Set(row.ExpiresAt),
			metadataSchedulerLeases.createdAt.Set(row.CreatedAt),
			metadataSchedulerLeases.updatedAt.Set(row.UpdatedAt),
		).
		OnConflict(metadataSchedulerLeases.taskName, metadataSchedulerLeases.scope).
		DoNothing()
	result, err := dbx.Exec(ctx, repository.store.dbxDB, query)
	if err != nil {
		return false, fmt.Errorf("insert scheduler lease: %w", err)
	}
	return hasAffectedRow(result, "insert scheduler lease")
}

func (repository *SQLSchedulerLeaseRepository) updateExpiredLease(
	ctx context.Context,
	lease scheduler.Lease,
	now int64,
) (bool, error) {
	query := querydsl.Update(metadataSchedulerLeases.schema).
		Set(
			metadataSchedulerLeases.owner.Set(lease.Owner),
			metadataSchedulerLeases.expiresAt.Set(lease.ExpiresAt.UTC().UnixNano()),
			metadataSchedulerLeases.updatedAt.Set(now),
		).
		Where(querydsl.And(
			metadataSchedulerLeases.taskName.Eq(lease.TaskName),
			metadataSchedulerLeases.scope.Eq(lease.Scope),
			metadataSchedulerLeases.expiresAt.Le(now),
		))
	result, err := dbx.Exec(ctx, repository.store.dbxDB, query)
	if err != nil {
		return false, fmt.Errorf("update expired scheduler lease: %w", err)
	}
	return hasAffectedRow(result, "update expired scheduler lease")
}

func (repository *SQLSchedulerLeaseRepository) renewOwnerLease(
	ctx context.Context,
	lease scheduler.Lease,
	now int64,
) (bool, error) {
	query := querydsl.Update(metadataSchedulerLeases.schema).
		Set(
			metadataSchedulerLeases.expiresAt.Set(lease.ExpiresAt.UTC().UnixNano()),
			metadataSchedulerLeases.updatedAt.Set(now),
		).
		Where(querydsl.And(
			metadataSchedulerLeases.taskName.Eq(lease.TaskName),
			metadataSchedulerLeases.scope.Eq(lease.Scope),
			metadataSchedulerLeases.owner.Eq(lease.Owner),
			metadataSchedulerLeases.expiresAt.Gt(now),
		))
	result, err := dbx.Exec(ctx, repository.store.dbxDB, query)
	if err != nil {
		return false, fmt.Errorf("renew scheduler lease: %w", err)
	}
	return hasAffectedRow(result, "renew scheduler lease")
}

func (repository *SQLSchedulerLeaseRepository) deleteOwnerLease(
	ctx context.Context,
	lease scheduler.Lease,
	now int64,
) (bool, error) {
	query := querydsl.DeleteFrom(metadataSchedulerLeases.schema).
		Where(querydsl.And(
			metadataSchedulerLeases.taskName.Eq(lease.TaskName),
			metadataSchedulerLeases.scope.Eq(lease.Scope),
			metadataSchedulerLeases.owner.Eq(lease.Owner),
			metadataSchedulerLeases.expiresAt.Gt(now),
		))
	result, err := dbx.Exec(ctx, repository.store.dbxDB, query)
	if err != nil {
		return false, fmt.Errorf("delete scheduler lease: %w", err)
	}
	return hasAffectedRow(result, "delete scheduler lease")
}

func (repository *SQLSchedulerLeaseRepository) deleteExpiredLease(
	ctx context.Context,
	taskName string,
	scope string,
	now int64,
) error {
	query := querydsl.DeleteFrom(metadataSchedulerLeases.schema).
		Where(querydsl.And(
			metadataSchedulerLeases.taskName.Eq(taskName),
			metadataSchedulerLeases.scope.Eq(scope),
			metadataSchedulerLeases.expiresAt.Le(now),
		))
	if _, err := dbx.Exec(ctx, repository.store.dbxDB, query); err != nil {
		return fmt.Errorf("delete expired scheduler lease: %w", err)
	}
	return nil
}

func (repository *SQLSchedulerLeaseRepository) getLease(
	ctx context.Context,
	taskName string,
	scope string,
) (scheduler.Lease, bool, error) {
	option, err := repositoryx.Query(repository.store.repos.schedulerLeases).
		Where(
			querydsl.And(
				metadataSchedulerLeases.taskName.Eq(taskName),
				metadataSchedulerLeases.scope.Eq(scope),
			),
		).
		Find(ctx)
	if err != nil {
		return scheduler.Lease{}, false, fmt.Errorf("query scheduler lease: %w", err)
	}
	row, found := option.Get()
	if !found {
		return scheduler.Lease{}, false, nil
	}
	return row.schedulerLease(), true, nil
}
