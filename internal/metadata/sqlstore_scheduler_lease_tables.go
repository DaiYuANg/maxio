package metadata

import (
	columnx "github.com/arcgolabs/dbx/column"
	schemax "github.com/arcgolabs/dbx/schema"
)

var metadataSchedulerLeases = newMetadataSchedulerLeasesTable()

type metadataSchedulerLease struct {
	TaskName  string `dbx:"task_name"`
	Scope     string `dbx:"scope"`
	Owner     string `dbx:"owner"`
	ExpiresAt int64  `dbx:"expires_at"`
	CreatedAt int64  `dbx:"created_at"`
	UpdatedAt int64  `dbx:"updated_at"`
}

type metadataSchedulerLeasesTable struct {
	schema    metadataSchedulerLeasesSchema
	taskName  columnx.Column[metadataSchedulerLease, string]
	scope     columnx.Column[metadataSchedulerLease, string]
	owner     columnx.Column[metadataSchedulerLease, string]
	expiresAt columnx.Column[metadataSchedulerLease, int64]
	createdAt columnx.Column[metadataSchedulerLease, int64]
	updatedAt columnx.Column[metadataSchedulerLease, int64]
}

type metadataSchedulerLeasesSchema struct {
	schemax.Schema[metadataSchedulerLease]
	TaskName  columnx.Column[metadataSchedulerLease, string] `dbx:"task_name,pk"`
	Scope     columnx.Column[metadataSchedulerLease, string] `dbx:"scope,pk"`
	Owner     columnx.Column[metadataSchedulerLease, string] `dbx:"owner"`
	ExpiresAt columnx.Column[metadataSchedulerLease, int64]  `dbx:"expires_at"`
	CreatedAt columnx.Column[metadataSchedulerLease, int64]  `dbx:"created_at"`
	UpdatedAt columnx.Column[metadataSchedulerLease, int64]  `dbx:"updated_at"`
}

func newMetadataSchedulerLeasesTable() metadataSchedulerLeasesTable {
	schema := schemax.MustSchema("metadata_scheduler_leases", metadataSchedulerLeasesSchema{})
	return metadataSchedulerLeasesTable{
		schema:    schema,
		taskName:  schema.TaskName,
		scope:     schema.Scope,
		owner:     schema.Owner,
		expiresAt: schema.ExpiresAt,
		createdAt: schema.CreatedAt,
		updatedAt: schema.UpdatedAt,
	}
}
