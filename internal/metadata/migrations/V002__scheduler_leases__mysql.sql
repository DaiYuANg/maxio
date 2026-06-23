CREATE TABLE IF NOT EXISTS metadata_scheduler_leases (
	task_name VARCHAR(255) NOT NULL,
	scope VARCHAR(255) NOT NULL,
	owner VARCHAR(255) NOT NULL,
	expires_at BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	PRIMARY KEY(task_name, scope)
);

CREATE INDEX idx_metadata_scheduler_leases_expires_at ON metadata_scheduler_leases (expires_at);

