package object

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/engine"
	"github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/lyonbrown4d/maxio/internal/store"
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
type RecoveryResult = store.RecoveryResult
type RecoveryPlan = store.RecoveryPlan
type RecoveryStatus = store.RecoveryStatus

type PutOptions = store.PutOptions

type Service struct {
	logger  *slog.Logger
	store   *store.Store
	search  *index.SearchEngine
	bus     eventx.BusRuntime
	cfg     config.Config
	indexMu sync.RWMutex
	index   IndexStatus
	indexCh chan indexTask
	indexWg sync.WaitGroup
}

func NewService(
	storage *store.Store,
	search *index.SearchEngine,
	bus eventx.BusRuntime,
	logger *slog.Logger,
	cfg config.Config,
) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		logger: logger,
		store:  storage,
		search: search,
		bus:    bus,
		cfg:    cfg,
	}
}

func (s *Service) ListBuckets(ctx context.Context) ([]Bucket, error) {
	buckets, err := s.store.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	return buckets, nil
}

func (s *Service) CreateBucket(ctx context.Context, name string) error {
	if err := s.store.CreateBucket(ctx, name); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	return nil
}

func (s *Service) DeleteBucket(ctx context.Context, name string) error {
	if err := s.store.DeleteBucket(ctx, name); err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	return nil
}

func (s *Service) ListObjects(ctx context.Context, bucket, prefix string) ([]ObjectMeta, error) {
	objects, err := s.store.ListObjects(ctx, bucket, prefix)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	return objects, nil
}

func (s *Service) PutObject(ctx context.Context, bucket, key string, reader io.Reader, opts PutOptions) (ObjectMeta, error) {
	meta, err := s.store.PutObject(ctx, bucket, key, reader, opts)
	if err != nil {
		return ObjectMeta{}, fmt.Errorf("put object: %w", err)
	}
	if err := s.publishObjectEvent(ctx, "object.updated", meta); err != nil {
		s.logger.WarnContext(ctx, "publish object event failed", "event", "object.updated", "error", err)
	}
	return meta, nil
}

func (s *Service) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectMeta, error) {
	body, meta, err := s.store.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, ObjectMeta{}, fmt.Errorf("get object: %w", err)
	}
	return body, meta, nil
}

func (s *Service) StatObject(ctx context.Context, bucket, key string) (ObjectMeta, error) {
	meta, err := s.store.StatObject(ctx, bucket, key)
	if err != nil {
		return ObjectMeta{}, fmt.Errorf("stat object: %w", err)
	}
	return meta, nil
}

func (s *Service) DeleteObject(ctx context.Context, bucket, key string) (ObjectMeta, error) {
	meta, err := s.store.DeleteObject(ctx, bucket, key)
	if err != nil {
		return ObjectMeta{}, fmt.Errorf("delete object: %w", err)
	}
	if err := s.publishObjectEvent(ctx, "object.deleted", meta); err != nil {
		s.logger.WarnContext(ctx, "publish object event failed", "event", "object.deleted", "error", err)
	}
	return meta, nil
}

func (s *Service) CheckHealth(ctx context.Context, bucket, key string) (Health, error) {
	health, err := s.store.CheckHealth(ctx, bucket, key)
	if err != nil {
		return Health{}, fmt.Errorf("check object health: %w", err)
	}
	return health, nil
}

func (s *Service) ScrubObject(ctx context.Context, bucket, key string) (ScrubResult, error) {
	result, err := s.store.ScrubObject(ctx, bucket, key)
	if err != nil {
		return ScrubResult{}, fmt.Errorf("scrub object: %w", err)
	}
	return result, nil
}

func (s *Service) RepairObject(ctx context.Context, bucket, key string) (RepairResult, error) {
	result, err := s.store.RepairObject(ctx, bucket, key)
	if err != nil {
		return RepairResult{}, fmt.Errorf("repair object: %w", err)
	}
	return result, nil
}

func (s *Service) PlanDedupe(ctx context.Context) (DedupeResult, error) {
	result, err := s.store.Dedupe(ctx, store.DedupeOptions{
		DryRun:   true,
		MaxFixes: s.cfg.DedupeMaxFixes,
	})
	if err != nil {
		return DedupeResult{}, fmt.Errorf("plan dedupe: %w", err)
	}
	return result, nil
}

func (s *Service) RunDedupe(ctx context.Context) (DedupeResult, error) {
	result, err := s.store.Dedupe(ctx, store.DedupeOptions{
		MaxFixes: s.cfg.DedupeMaxFixes,
	})
	if err != nil {
		return DedupeResult{}, fmt.Errorf("run dedupe: %w", err)
	}
	return result, nil
}

func (s *Service) RebalanceNode(ctx context.Context, nodeID string) (RebalanceResult, error) {
	result, err := s.store.RebalanceNode(ctx, nodeID)
	if err != nil {
		return RebalanceResult{}, fmt.Errorf("rebalance node: %w", err)
	}
	return result, nil
}

func (s *Service) Recover(ctx context.Context) (RecoveryResult, error) {
	result, err := s.store.Recover(ctx, store.RecoveryOptions{
		PendingTTL:          s.cfg.PendingObjectTTLDuration(),
		CleanupOrphanShards: true,
	})
	if err != nil {
		return RecoveryResult{}, fmt.Errorf("recover storage: %w", err)
	}
	return result, nil
}

func (s *Service) RecoveryPlan(ctx context.Context) (RecoveryPlan, error) {
	plan, err := s.store.PlanRecovery(ctx, s.cfg.PendingObjectTTLDuration())
	if err != nil {
		return RecoveryPlan{}, fmt.Errorf("plan recovery: %w", err)
	}
	return plan, nil
}

func (s *Service) RecoveryStatus() RecoveryStatus {
	if s == nil || s.store == nil {
		return RecoveryStatus{}
	}
	return s.store.RecoveryStatus()
}

func (s *Service) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	_ = ctx
	return s.search.Search(query), nil
}

func (s *Service) IndexStatus() IndexStatus {
	if s == nil {
		return IndexStatus{}
	}
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()
	status := s.index
	if s.indexCh != nil {
		status.QueuedObjects = len(s.indexCh)
		status.QueueSize = cap(s.indexCh)
	}
	return status
}

func (s *Service) publishObjectEvent(ctx context.Context, eventType string, meta ObjectMeta) error {
	if s.bus == nil {
		return nil
	}
	if err := s.bus.Publish(ctx, ObjectEvent{
		Event:   eventType,
		Payload: objectEventPayload(meta),
	}); err != nil {
		return fmt.Errorf("publish object event: %w", err)
	}
	return nil
}

type ObjectEvent struct {
	Event   string
	Payload map[string]any
}

func (e ObjectEvent) Name() string {
	return e.Event
}
