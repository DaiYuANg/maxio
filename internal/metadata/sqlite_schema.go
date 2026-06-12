package metadata

var sqliteSchemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS metadata_buckets (
		name TEXT PRIMARY KEY,
		created_at INTEGER NOT NULL
	)`,
	`CREATE TABLE IF NOT EXISTS metadata_objects (
		bucket TEXT NOT NULL,
		object_key TEXT NOT NULL,
		hash TEXT NOT NULL,
		etag TEXT NOT NULL,
		size INTEGER NOT NULL,
		content_type TEXT NOT NULL,
		cache_control TEXT NOT NULL,
		content_disposition TEXT NOT NULL,
		content_encoding TEXT NOT NULL,
		content_language TEXT NOT NULL,
		user_metadata TEXT,
		updated_at INTEGER NOT NULL,
		state TEXT NOT NULL,
		write_intent_id TEXT,
		write_intent_stage TEXT,
		write_intent_started_at INTEGER,
		write_intent_updated_at INTEGER,
		shard_placements TEXT,
		shard_checksums TEXT,
		shard_sizes TEXT,
		PRIMARY KEY(bucket, object_key)
	)`,
	`CREATE TABLE IF NOT EXISTS metadata_blob_refs (
		hash TEXT PRIMARY KEY,
		path TEXT NOT NULL,
		size INTEGER NOT NULL,
		ref_count INTEGER NOT NULL,
		shard_placements TEXT,
		shard_checksums TEXT,
		shard_sizes TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_metadata_objects_bucket_key ON metadata_objects (bucket, object_key)`,
	`CREATE INDEX IF NOT EXISTS idx_metadata_objects_bucket_state ON metadata_objects (bucket, state)`,
	`CREATE INDEX IF NOT EXISTS idx_metadata_objects_state ON metadata_objects (state)`,
}
