CREATE TABLE IF NOT EXISTS metadata_processing_records (
	record_id TEXT PRIMARY KEY,
	bucket TEXT NOT NULL,
	object_key TEXT NOT NULL,
	version_id TEXT NOT NULL,
	digest TEXT NOT NULL,
	mode TEXT NOT NULL,
	status TEXT NOT NULL,
	error TEXT NOT NULL,
	results TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_metadata_processing_records_status ON metadata_processing_records (status, updated_at);

