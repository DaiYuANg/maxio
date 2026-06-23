CREATE TABLE IF NOT EXISTS metadata_scheduler_leases (
	task_name TEXT NOT NULL,
	scope TEXT NOT NULL,
	owner TEXT NOT NULL,
	expires_at INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	PRIMARY KEY(task_name, scope)
);

CREATE INDEX IF NOT EXISTS idx_metadata_scheduler_leases_expires_at ON metadata_scheduler_leases (expires_at);
