// Package model contains shared MaxIO data models.
package model

import "time"

const (
	ObjectStatePending   = "pending"
	ObjectStateCommitted = "committed"
)

type Bucket struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Upstream struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Endpoint  string    `json:"endpoint"`
	Region    string    `json:"region,omitempty"`
	Weight    int       `json:"weight"`
	Priority  int       `json:"priority"`
	Buckets   []string  `json:"buckets,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ObjectMeta struct {
	Bucket             string            `json:"bucket"`
	Key                string            `json:"key"`
	Hash               string            `json:"hash"`
	ETag               string            `json:"etag"`
	Size               int64             `json:"size"`
	ContentType        string            `json:"content_type"`
	CacheControl       string            `json:"cache_control,omitempty"`
	ContentDisposition string            `json:"content_disposition,omitempty"`
	ContentEncoding    string            `json:"content_encoding,omitempty"`
	ContentLanguage    string            `json:"content_language,omitempty"`
	UserMetadata       map[string]string `json:"user_metadata,omitempty"`
	UpdatedAt          time.Time         `json:"updated_at"`
	State              string            `json:"state,omitempty"`
	WriteIntent        *WriteIntent      `json:"write_intent,omitempty"`
}

type ObjectRecord struct {
	Bucket           string    `json:"bucket"`
	Key              string    `json:"key"`
	CurrentVersionID string    `json:"current_version_id,omitempty"`
	Deleted          bool      `json:"deleted,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ObjectVersion struct {
	Bucket             string            `json:"bucket"`
	Key                string            `json:"key"`
	VersionID          string            `json:"version_id"`
	Digest             string            `json:"digest,omitempty"`
	ETag               string            `json:"etag,omitempty"`
	Size               int64             `json:"size"`
	ContentType        string            `json:"content_type,omitempty"`
	CacheControl       string            `json:"cache_control,omitempty"`
	ContentDisposition string            `json:"content_disposition,omitempty"`
	ContentEncoding    string            `json:"content_encoding,omitempty"`
	ContentLanguage    string            `json:"content_language,omitempty"`
	UserMetadata       map[string]string `json:"user_metadata,omitempty"`
	UpstreamID         string            `json:"upstream_id,omitempty"`
	UpstreamBucket     string            `json:"upstream_bucket,omitempty"`
	UpstreamKey        string            `json:"upstream_key,omitempty"`
	DeleteMarker       bool              `json:"delete_marker,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

type DigestRef struct {
	Digest         string    `json:"digest"`
	Size           int64     `json:"size"`
	RefCount       int       `json:"ref_count"`
	UpstreamID     string    `json:"upstream_id,omitempty"`
	UpstreamBucket string    `json:"upstream_bucket,omitempty"`
	UpstreamKey    string    `json:"upstream_key,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type WriteIntent struct {
	ID        string    `json:"id"`
	Stage     string    `json:"stage"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	WriteIntentStageUnknown        = "unknown"
	WriteIntentStageMetadataStaged = "metadata_staged"
	WriteIntentStageBlobPrepared   = "blob_prepared"
	WriteIntentStageLayoutLinked   = "layout_linked"
	WriteIntentStageBlobRetained   = "blob_retained"
	WriteIntentStageCommitted      = "committed"
)

type SearchQuery struct {
	Query        string `json:"q,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Prefix       string `json:"prefix,omitempty"`
	NameContains string `json:"name_contains,omitempty"`
	ContentType  string `json:"content_type,omitempty"`
	Limit        int    `json:"limit,omitempty"`
	MinSize      int64  `json:"min_size,omitempty"`
	MaxSize      int64  `json:"max_size,omitempty"`
}

type SearchResult struct {
	Items []ObjectMeta `json:"items"`
}

const (
	IndexDocumentStatePending = "pending"
	IndexDocumentStateIndexed = "indexed"
	IndexDocumentStateDeleted = "deleted"
	IndexDocumentStateFailed  = "failed"

	IndexJobKindUpsert  = "upsert"
	IndexJobKindDelete  = "delete"
	IndexJobKindRebuild = "rebuild"

	IndexJobStatusQueued    = "queued"
	IndexJobStatusRunning   = "running"
	IndexJobStatusSucceeded = "succeeded"
	IndexJobStatusFailed    = "failed"

	IndexOutboxStatusPending    = "pending"
	IndexOutboxStatusDispatched = "dispatched"
	IndexOutboxStatusFailed     = "failed"
)

type IndexDocument struct {
	ID        string    `json:"id"`
	Bucket    string    `json:"bucket"`
	Key       string    `json:"key"`
	VersionID string    `json:"version_id"`
	Digest    string    `json:"digest,omitempty"`
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	IndexedAt time.Time `json:"indexed_at,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type IndexJob struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Bucket      string    `json:"bucket,omitempty"`
	Key         string    `json:"key,omitempty"`
	VersionID   string    `json:"version_id,omitempty"`
	Status      string    `json:"status"`
	Attempts    int       `json:"attempts"`
	Error       string    `json:"error,omitempty"`
	AvailableAt time.Time `json:"available_at"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type IndexOutboxEvent struct {
	ID          string    `json:"id"`
	EventType   string    `json:"event_type"`
	Bucket      string    `json:"bucket,omitempty"`
	Key         string    `json:"key,omitempty"`
	VersionID   string    `json:"version_id,omitempty"`
	Payload     string    `json:"payload,omitempty"`
	Status      string    `json:"status"`
	Attempts    int       `json:"attempts"`
	Error       string    `json:"error,omitempty"`
	AvailableAt time.Time `json:"available_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
