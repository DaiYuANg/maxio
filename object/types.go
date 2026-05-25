package object

import (
	"context"
	"io"
	"time"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/index"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
)

var (
	ErrNotFound            = store.ErrNotFound
	ErrBucketExists        = store.ErrBucketExists
	ErrBucketNotFound      = store.ErrBucketNotFound
	ErrBadRequest          = store.ErrBadRequest
	ErrEngineFailed        = store.ErrEngineFailed
	ErrObjectCorrupted     = engine.ErrObjectCorrupted
	ErrShardRecoveryFailed = engine.ErrShardRecoveryFailed
)

type Bucket = model.Bucket
type ObjectMeta = model.ObjectMeta
type SearchQuery = model.SearchQuery
type SearchResult = model.SearchResult
type Health = engine.Health
type RepairResult = engine.RepairResult
type ScrubResult = engine.ScrubResult
type DedupeOptions = store.DedupeOptions
type DedupeResult = store.DedupeResult
type RebalanceResult = store.RebalanceResult
type RecoveryOptions = store.RecoveryOptions
type RecoveryResult = store.RecoveryResult
type RecoveryPlan = store.RecoveryPlan
type RecoveryStatus = store.RecoveryStatus
type PutOptions = store.PutOptions

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
	PruneExcept(valid []ObjectMeta) error
}
