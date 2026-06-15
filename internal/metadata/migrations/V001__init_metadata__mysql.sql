CREATE TABLE IF NOT EXISTS metadata_upstreams (
	id VARCHAR(255) PRIMARY KEY,
	name VARCHAR(255) NOT NULL,
	endpoint TEXT NOT NULL,
	region VARCHAR(255) NOT NULL,
	weight INTEGER NOT NULL,
	priority INTEGER NOT NULL,
	buckets TEXT,
	enabled INTEGER NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata_buckets (
	name VARBINARY(255) PRIMARY KEY,
	created_at BIGINT NOT NULL
);

CREATE INDEX idx_metadata_upstreams_enabled_priority ON metadata_upstreams (enabled, priority, name);

CREATE TABLE IF NOT EXISTS metadata_objects (
	bucket VARBINARY(255) NOT NULL,
	object_key VARBINARY(1024) NOT NULL,
	hash VARCHAR(255) NOT NULL,
	etag VARCHAR(255) NOT NULL,
	size BIGINT NOT NULL,
	content_type TEXT NOT NULL,
	cache_control TEXT NOT NULL,
	content_disposition TEXT NOT NULL,
	content_encoding TEXT NOT NULL,
	content_language TEXT NOT NULL,
	user_metadata TEXT,
	updated_at BIGINT NOT NULL,
	state VARCHAR(64) NOT NULL,
	write_intent_id VARCHAR(255),
	write_intent_stage VARCHAR(255),
	write_intent_started_at BIGINT,
	write_intent_updated_at BIGINT,
	PRIMARY KEY(bucket, object_key)
);

CREATE TABLE IF NOT EXISTS metadata_blob_refs (
	hash VARCHAR(255) PRIMARY KEY,
	path TEXT NOT NULL,
	size BIGINT NOT NULL,
	ref_count INTEGER NOT NULL
);

CREATE INDEX idx_metadata_objects_bucket_key ON metadata_objects (bucket, object_key);
CREATE INDEX idx_metadata_objects_bucket_state ON metadata_objects (bucket, state);
CREATE INDEX idx_metadata_objects_state ON metadata_objects (state);

CREATE TABLE IF NOT EXISTS metadata_object_records (
	bucket VARBINARY(255) NOT NULL,
	object_key VARBINARY(1024) NOT NULL,
	current_version_id VARCHAR(255) NOT NULL,
	deleted INTEGER NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	PRIMARY KEY(bucket, object_key)
);

CREATE TABLE IF NOT EXISTS metadata_object_versions (
	bucket VARBINARY(255) NOT NULL,
	object_key VARBINARY(1024) NOT NULL,
	version_id VARCHAR(255) NOT NULL,
	digest VARCHAR(255) NOT NULL,
	etag VARCHAR(255) NOT NULL,
	size BIGINT NOT NULL,
	content_type TEXT NOT NULL,
	cache_control TEXT NOT NULL,
	content_disposition TEXT NOT NULL,
	content_encoding TEXT NOT NULL,
	content_language TEXT NOT NULL,
	user_metadata TEXT,
	upstream_id VARCHAR(255) NOT NULL,
	upstream_bucket VARBINARY(255) NOT NULL,
	upstream_key VARBINARY(1024) NOT NULL,
	delete_marker INTEGER NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	PRIMARY KEY(bucket, object_key, version_id)
);

CREATE TABLE IF NOT EXISTS metadata_digest_refs (
	digest VARCHAR(255) PRIMARY KEY,
	size BIGINT NOT NULL,
	ref_count INTEGER NOT NULL,
	upstream_id VARCHAR(255) NOT NULL,
	upstream_bucket VARBINARY(255) NOT NULL,
	upstream_key VARBINARY(1024) NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata_index_documents (
	document_id VARCHAR(255) PRIMARY KEY,
	bucket VARBINARY(255) NOT NULL,
	object_key VARBINARY(1024) NOT NULL,
	version_id VARCHAR(255) NOT NULL,
	digest VARCHAR(255) NOT NULL,
	state VARCHAR(64) NOT NULL,
	error TEXT NOT NULL,
	indexed_at BIGINT,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata_index_jobs (
	job_id VARCHAR(255) PRIMARY KEY,
	kind VARCHAR(64) NOT NULL,
	bucket VARBINARY(255) NOT NULL,
	object_key VARBINARY(1024) NOT NULL,
	version_id VARCHAR(255) NOT NULL,
	status VARCHAR(64) NOT NULL,
	attempts INTEGER NOT NULL,
	error TEXT NOT NULL,
	available_at BIGINT NOT NULL,
	started_at BIGINT,
	finished_at BIGINT,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata_index_outbox (
	event_id VARCHAR(255) PRIMARY KEY,
	event_type VARCHAR(255) NOT NULL,
	bucket VARBINARY(255) NOT NULL,
	object_key VARBINARY(1024) NOT NULL,
	version_id VARCHAR(255) NOT NULL,
	payload TEXT NOT NULL,
	status VARCHAR(64) NOT NULL,
	attempts INTEGER NOT NULL,
	error TEXT NOT NULL,
	available_at BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE INDEX idx_metadata_object_records_bucket_key ON metadata_object_records (bucket, object_key);
CREATE INDEX idx_metadata_object_versions_object ON metadata_object_versions (bucket, object_key, created_at);
CREATE INDEX idx_metadata_object_versions_digest ON metadata_object_versions (digest);
CREATE INDEX idx_metadata_digest_refs_ref_count ON metadata_digest_refs (ref_count);
CREATE INDEX idx_metadata_index_documents_object ON metadata_index_documents (bucket, object_key, version_id);
CREATE INDEX idx_metadata_index_documents_state ON metadata_index_documents (state);
CREATE INDEX idx_metadata_index_jobs_status_available ON metadata_index_jobs (status, available_at, created_at);
CREATE INDEX idx_metadata_index_outbox_status_available ON metadata_index_outbox (status, available_at, created_at);
