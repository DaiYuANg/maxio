package object

import (
	"context"
	"errors"
	"io"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var (
	ErrNotFound             = errors.New("object not found")
	ErrBucketExists         = errors.New("bucket already exists")
	ErrBucketNotFound       = errors.New("bucket not found")
	ErrBadRequest           = errors.New("bad request")
	ErrDataPlaneUnavailable = errors.New("native object data plane is unavailable in s3 proxy mode")
	ErrEngineFailed         = ErrDataPlaneUnavailable
	ErrObjectCorrupted      = errors.New("object corrupted")
)

type Bucket = model.Bucket
type ObjectMeta = model.ObjectMeta
type SearchQuery = model.SearchQuery
type SearchResult = model.SearchResult

type Health struct {
	Healthy bool `json:"healthy"`
}

type RepairResult struct {
	Repaired bool `json:"repaired"`
}

type ScrubResult struct {
	Healthy bool `json:"healthy"`
}

type DedupeOptions struct {
	DryRun   bool `json:"dry_run,omitempty"`
	MaxFixes int  `json:"max_fixes,omitempty"`
}

type DedupeResult struct {
	Objects           int   `json:"objects"`
	BlobRefs          int   `json:"blob_refs"`
	Hashes            int   `json:"hashes"`
	Fixes             int   `json:"fixes"`
	RefCountDrift     int   `json:"ref_count_drift"`
	MissingBlobRefs   int   `json:"missing_blob_refs"`
	OrphanBlobRefs    int   `json:"orphan_blob_refs"`
	LayoutsMismatched int   `json:"layouts_mismatched"`
	BytesReclaimable  int64 `json:"bytes_reclaimable"`
	BytesReclaimed    int64 `json:"bytes_reclaimed"`
	Limited           bool  `json:"limited"`
}

type RebalanceResult struct {
	Objects   int   `json:"objects"`
	UsedBytes int64 `json:"used_bytes"`
}

type RecoveryOptions struct {
	PendingTTL time.Duration `json:"-"`
}

type RecoveryResult struct {
	DryRun         bool           `json:"dry_run"`
	PendingRemoved int            `json:"pending_removed"`
	PendingActions map[string]int `json:"pending_actions,omitempty"`
}

type RecoveryPlan struct {
	PendingActions map[string]int `json:"pending_actions,omitempty"`
}

type RecoveryStatus struct {
	Completed  bool           `json:"completed"`
	LastError  string         `json:"last_error,omitempty"`
	LastResult RecoveryResult `json:"last_result"`
}

type PutOptions struct {
	ContentType        string
	CacheControl       string
	ContentDisposition string
	ContentEncoding    string
	ContentLanguage    string
	UserMetadata       map[string]string
}

type Store interface {
	ListBuckets(ctx context.Context) ([]Bucket, error)
	CreateBucket(ctx context.Context, name string) error
	DeleteBucket(ctx context.Context, name string) error
	ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectMeta, error)
	PutObject(ctx context.Context, bucket, key string, reader io.Reader, opts PutOptions) (ObjectMeta, error)
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectMeta, error)
	StatObject(ctx context.Context, bucket, key string) (ObjectMeta, error)
	DeleteObject(ctx context.Context, bucket, key string) (ObjectMeta, error)
	CheckHealth(ctx context.Context, bucket, key string) (Health, error)
	ScrubObject(ctx context.Context, bucket, key string) (ScrubResult, error)
	RepairObject(ctx context.Context, bucket, key string) (RepairResult, error)
	Dedupe(ctx context.Context, opts DedupeOptions) (DedupeResult, error)
	RebalanceNode(ctx context.Context, nodeID string) (RebalanceResult, error)
	Recover(ctx context.Context, opts RecoveryOptions) (RecoveryResult, error)
	PlanRecovery(ctx context.Context, pendingTTL time.Duration) (RecoveryPlan, error)
	RecoveryStatus() RecoveryStatus
}

type SearchIndex interface {
	Search(query SearchQuery) SearchResult
	UpsertDocument(meta ObjectMeta, text string)
	UpsertDocuments(docs []index.IndexDocument) (int, error)
	Remove(bucket, key string)
	PruneExcept(valid *collectionlist.List[ObjectMeta]) error
}
