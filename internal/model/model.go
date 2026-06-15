// Package model contains shared MaxIO data models.
package model

import "time"

const (
	ObjectStatePending   = "pending"
	ObjectStateCommitted = "committed"
)

type Bucket struct {
	Name      string    `dbx:"name"                            json:"name"`
	CreatedAt time.Time `dbx:"created_at,codec=unix_nano_time" json:"created_at"`
}

type Upstream struct {
	ID        string    `dbx:"id"                              json:"id"`
	Name      string    `dbx:"name"                            json:"name"`
	Endpoint  string    `dbx:"endpoint"                        json:"endpoint"`
	Region    string    `dbx:"region"                          json:"region,omitempty"`
	Weight    int       `dbx:"weight"                          json:"weight"`
	Priority  int       `dbx:"priority"                        json:"priority"`
	Buckets   []string  `dbx:"buckets,codec=json"              json:"buckets,omitempty"`
	Enabled   bool      `dbx:"enabled,codec=bool_int"          json:"enabled"`
	CreatedAt time.Time `dbx:"created_at,codec=unix_nano_time" json:"created_at"`
	UpdatedAt time.Time `dbx:"updated_at,codec=unix_nano_time" json:"updated_at"`
}

type ObjectMeta struct {
	Bucket             string            `dbx:"bucket"                          json:"bucket"`
	Key                string            `dbx:"object_key"                      json:"key"`
	Hash               string            `dbx:"hash"                            json:"hash"`
	ETag               string            `dbx:"etag"                            json:"etag"`
	Size               int64             `dbx:"size"                            json:"size"`
	ContentType        string            `dbx:"content_type"                    json:"content_type"`
	CacheControl       string            `dbx:"cache_control"                   json:"cache_control,omitempty"`
	ContentDisposition string            `dbx:"content_disposition"             json:"content_disposition,omitempty"`
	ContentEncoding    string            `dbx:"content_encoding"                json:"content_encoding,omitempty"`
	ContentLanguage    string            `dbx:"content_language"                json:"content_language,omitempty"`
	UserMetadata       map[string]string `dbx:"user_metadata,codec=json"        json:"user_metadata,omitempty"`
	UpdatedAt          time.Time         `dbx:"updated_at,codec=unix_nano_time" json:"updated_at"`
	State              string            `dbx:"state"                           json:"state,omitempty"`
	WriteIntent        *WriteIntent      `dbx:"-"                               json:"write_intent,omitempty"`
}

type ObjectRecord struct {
	Bucket           string    `dbx:"bucket"                          json:"bucket"`
	Key              string    `dbx:"object_key"                      json:"key"`
	CurrentVersionID string    `dbx:"current_version_id"              json:"current_version_id,omitempty"`
	Deleted          bool      `dbx:"deleted,codec=bool_int"          json:"deleted,omitempty"`
	CreatedAt        time.Time `dbx:"created_at,codec=unix_nano_time" json:"created_at"`
	UpdatedAt        time.Time `dbx:"updated_at,codec=unix_nano_time" json:"updated_at"`
}

type ObjectVersion struct {
	Bucket             string            `dbx:"bucket"                          json:"bucket"`
	Key                string            `dbx:"object_key"                      json:"key"`
	VersionID          string            `dbx:"version_id"                      json:"version_id"`
	Digest             string            `dbx:"digest"                          json:"digest,omitempty"`
	ETag               string            `dbx:"etag"                            json:"etag,omitempty"`
	Size               int64             `dbx:"size"                            json:"size"`
	ContentType        string            `dbx:"content_type"                    json:"content_type,omitempty"`
	CacheControl       string            `dbx:"cache_control"                   json:"cache_control,omitempty"`
	ContentDisposition string            `dbx:"content_disposition"             json:"content_disposition,omitempty"`
	ContentEncoding    string            `dbx:"content_encoding"                json:"content_encoding,omitempty"`
	ContentLanguage    string            `dbx:"content_language"                json:"content_language,omitempty"`
	UserMetadata       map[string]string `dbx:"user_metadata,codec=json"        json:"user_metadata,omitempty"`
	UpstreamID         string            `dbx:"upstream_id"                     json:"upstream_id,omitempty"`
	UpstreamBucket     string            `dbx:"upstream_bucket"                 json:"upstream_bucket,omitempty"`
	UpstreamKey        string            `dbx:"upstream_key"                    json:"upstream_key,omitempty"`
	DeleteMarker       bool              `dbx:"delete_marker,codec=bool_int"    json:"delete_marker,omitempty"`
	CreatedAt          time.Time         `dbx:"created_at,codec=unix_nano_time" json:"created_at"`
	UpdatedAt          time.Time         `dbx:"updated_at,codec=unix_nano_time" json:"updated_at"`
}

type DigestRef struct {
	Digest         string    `dbx:"digest"                          json:"digest"`
	Size           int64     `dbx:"size"                            json:"size"`
	RefCount       int       `dbx:"ref_count"                       json:"ref_count"`
	UpstreamID     string    `dbx:"upstream_id"                     json:"upstream_id,omitempty"`
	UpstreamBucket string    `dbx:"upstream_bucket"                 json:"upstream_bucket,omitempty"`
	UpstreamKey    string    `dbx:"upstream_key"                    json:"upstream_key,omitempty"`
	CreatedAt      time.Time `dbx:"created_at,codec=unix_nano_time" json:"created_at"`
	UpdatedAt      time.Time `dbx:"updated_at,codec=unix_nano_time" json:"updated_at"`
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
	ID        string    `dbx:"document_id"                     json:"id"`
	Bucket    string    `dbx:"bucket"                          json:"bucket"`
	Key       string    `dbx:"object_key"                      json:"key"`
	VersionID string    `dbx:"version_id"                      json:"version_id"`
	Digest    string    `dbx:"digest"                          json:"digest,omitempty"`
	State     string    `dbx:"state"                           json:"state"`
	Error     string    `dbx:"error"                           json:"error,omitempty"`
	IndexedAt time.Time `dbx:"indexed_at,codec=unix_nano_time" json:"indexed_at,omitempty"`
	CreatedAt time.Time `dbx:"created_at,codec=unix_nano_time" json:"created_at"`
	UpdatedAt time.Time `dbx:"updated_at,codec=unix_nano_time" json:"updated_at"`
}

type IndexJob struct {
	ID          string    `dbx:"job_id"                            json:"id"`
	Kind        string    `dbx:"kind"                              json:"kind"`
	Bucket      string    `dbx:"bucket"                            json:"bucket,omitempty"`
	Key         string    `dbx:"object_key"                        json:"key,omitempty"`
	VersionID   string    `dbx:"version_id"                        json:"version_id,omitempty"`
	Status      string    `dbx:"status"                            json:"status"`
	Attempts    int       `dbx:"attempts"                          json:"attempts"`
	Error       string    `dbx:"error"                             json:"error,omitempty"`
	AvailableAt time.Time `dbx:"available_at,codec=unix_nano_time" json:"available_at"`
	StartedAt   time.Time `dbx:"started_at,codec=unix_nano_time"   json:"started_at,omitempty"`
	FinishedAt  time.Time `dbx:"finished_at,codec=unix_nano_time"  json:"finished_at,omitempty"`
	CreatedAt   time.Time `dbx:"created_at,codec=unix_nano_time"   json:"created_at"`
	UpdatedAt   time.Time `dbx:"updated_at,codec=unix_nano_time"   json:"updated_at"`
}

type IndexOutboxEvent struct {
	ID          string    `dbx:"event_id"                          json:"id"`
	EventType   string    `dbx:"event_type"                        json:"event_type"`
	Bucket      string    `dbx:"bucket"                            json:"bucket,omitempty"`
	Key         string    `dbx:"object_key"                        json:"key,omitempty"`
	VersionID   string    `dbx:"version_id"                        json:"version_id,omitempty"`
	Payload     string    `dbx:"payload"                           json:"payload,omitempty"`
	Status      string    `dbx:"status"                            json:"status"`
	Attempts    int       `dbx:"attempts"                          json:"attempts"`
	Error       string    `dbx:"error"                             json:"error,omitempty"`
	AvailableAt time.Time `dbx:"available_at,codec=unix_nano_time" json:"available_at"`
	CreatedAt   time.Time `dbx:"created_at,codec=unix_nano_time"   json:"created_at"`
	UpdatedAt   time.Time `dbx:"updated_at,codec=unix_nano_time"   json:"updated_at"`
}
