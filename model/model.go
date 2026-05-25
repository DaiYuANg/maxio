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

type ShardPlacement struct {
	Index       int    `json:"index"`
	NodeID      string `json:"node_id"`
	NodeAddress string `json:"node_address,omitempty"`
	Local       bool   `json:"local,omitempty"`
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
	ShardPlacements    []ShardPlacement  `json:"shard_placements,omitempty"`
	ShardChecksums     []string          `json:"shard_checksums,omitempty"`
	ShardSizes         []int64           `json:"shard_sizes,omitempty"`
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
