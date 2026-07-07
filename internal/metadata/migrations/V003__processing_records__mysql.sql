CREATE TABLE IF NOT EXISTS metadata_processing_records (
	record_id VARBINARY(2048) PRIMARY KEY,
	bucket VARBINARY(255) NOT NULL,
	object_key VARBINARY(1024) NOT NULL,
	version_id VARCHAR(255) NOT NULL,
	digest VARCHAR(255) NOT NULL,
	mode VARCHAR(64) NOT NULL,
	status VARCHAR(64) NOT NULL,
	error TEXT NOT NULL,
	results TEXT NOT NULL,
	created_at BIGINT NOT NULL,
	updated_at BIGINT NOT NULL
);

CREATE INDEX idx_metadata_processing_records_status ON metadata_processing_records (status, updated_at);

