CREATE TABLE IF NOT EXISTS metadata_upstreams (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	endpoint TEXT NOT NULL,
	region TEXT NOT NULL,
	weight INTEGER NOT NULL,
	priority INTEGER NOT NULL,
	buckets TEXT,
	enabled INTEGER NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata_buckets (
	name TEXT PRIMARY KEY,
	created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_metadata_upstreams_enabled_priority ON metadata_upstreams (enabled, priority, name);

CREATE TABLE IF NOT EXISTS metadata_objects (
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	hash TEXT NOT NULL,
	etag TEXT NOT NULL,
	size BIGINT NOT NULL,
	content_type TEXT NOT NULL,
	cache_control TEXT NOT NULL,
	content_disposition TEXT NOT NULL,
	content_encoding TEXT NOT NULL,
	content_language TEXT NOT NULL,
	user_metadata TEXT,
	updated_at BIGINT NOT NULL,
	state TEXT NOT NULL,
	write_intent_id TEXT,
	write_intent_stage TEXT,
	write_intent_started_at BIGINT,
	write_intent_updated_at BIGINT,
	shard_placements TEXT,
	shard_checksums TEXT,
	shard_sizes TEXT,
	PRIMARY KEY(bucket, object_key)
);

CREATE TABLE IF NOT EXISTS metadata_blob_refs (
	hash TEXT PRIMARY KEY,
	path TEXT NOT NULL,
	size BIGINT NOT NULL,
	ref_count INTEGER NOT NULL,
	shard_placements TEXT,
	shard_checksums TEXT,
	shard_sizes TEXT
);

CREATE INDEX IF NOT EXISTS idx_metadata_objects_bucket_key ON metadata_objects (bucket, object_key);
CREATE INDEX IF NOT EXISTS idx_metadata_objects_bucket_state ON metadata_objects (bucket, state);
CREATE INDEX IF NOT EXISTS idx_metadata_objects_state ON metadata_objects (state);

CREATE TABLE IF NOT EXISTS metadata_object_records (
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	current_version_id TEXT NOT NULL,
	deleted INTEGER NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	PRIMARY KEY(bucket, object_key)
);

CREATE TABLE IF NOT EXISTS metadata_object_versions (
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	version_id TEXT NOT NULL,
	digest TEXT NOT NULL,
	etag TEXT NOT NULL,
	size BIGINT NOT NULL,
	content_type TEXT NOT NULL,
	cache_control TEXT NOT NULL,
	content_disposition TEXT NOT NULL,
	content_encoding TEXT NOT NULL,
	content_language TEXT NOT NULL,
	user_metadata TEXT,
	upstream_id TEXT NOT NULL,
	upstream_bucket TEXT NOT NULL,
	upstream_key TEXT NOT NULL,
	delete_marker INTEGER NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL,
	PRIMARY KEY(bucket, object_key, version_id)
);

CREATE TABLE IF NOT EXISTS metadata_digest_refs (
	digest TEXT PRIMARY KEY,
	size BIGINT NOT NULL,
	ref_count INTEGER NOT NULL,
	upstream_id TEXT NOT NULL,
	upstream_bucket TEXT NOT NULL,
	upstream_key TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata_index_documents (
	document_id TEXT PRIMARY KEY,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	version_id TEXT NOT NULL,
	digest TEXT NOT NULL,
	state TEXT NOT NULL,
	error TEXT NOT NULL,
	indexed_at BIGINT,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata_index_jobs (
	job_id TEXT PRIMARY KEY,
	kind TEXT NOT NULL,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	version_id TEXT NOT NULL,
	status TEXT NOT NULL,
	attempts INTEGER NOT NULL,
	error TEXT NOT NULL,
	available_at BIGINT NOT NULL,
	started_at BIGINT,
	finished_at BIGINT,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE TABLE IF NOT EXISTS metadata_index_outbox (
	event_id TEXT PRIMARY KEY,
	event_type TEXT NOT NULL,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	version_id TEXT NOT NULL,
	payload TEXT NOT NULL,
	status TEXT NOT NULL,
	attempts INTEGER NOT NULL,
	error TEXT NOT NULL,
	available_at BIGINT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_metadata_object_records_bucket_key ON metadata_object_records (bucket, object_key);
CREATE INDEX IF NOT EXISTS idx_metadata_object_versions_object ON metadata_object_versions (bucket, object_key, created_at);
CREATE INDEX IF NOT EXISTS idx_metadata_object_versions_digest ON metadata_object_versions (digest);
CREATE INDEX IF NOT EXISTS idx_metadata_digest_refs_ref_count ON metadata_digest_refs (ref_count);
CREATE INDEX IF NOT EXISTS idx_metadata_index_documents_object ON metadata_index_documents (bucket, object_key, version_id);
CREATE INDEX IF NOT EXISTS idx_metadata_index_documents_state ON metadata_index_documents (state);
CREATE INDEX IF NOT EXISTS idx_metadata_index_jobs_status_available ON metadata_index_jobs (status, available_at, created_at);
CREATE INDEX IF NOT EXISTS idx_metadata_index_outbox_status_available ON metadata_index_outbox (status, available_at, created_at);
