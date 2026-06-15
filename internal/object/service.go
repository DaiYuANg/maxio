package object

import (
	"context"
	"io"
	"log/slog"
	"sync"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/eventx"
)

type Service struct {
	logger  *slog.Logger
	search  SearchIndex
	bus     eventx.BusRuntime
	cfg     Config
	indexMu sync.RWMutex
	index   IndexStatus
}

func NewService(_ Store, search SearchIndex, bus eventx.BusRuntime, logger *slog.Logger, cfg Config) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		logger: logger,
		search: search,
		bus:    bus,
		cfg:    cfg.normalized(),
	}
}

func (s *Service) ListBuckets(ctx context.Context) (*collectionlist.List[Bucket], error) {
	_ = ctx
	return nil, ErrDataPlaneUnavailable
}

func (s *Service) CreateBucket(ctx context.Context, name string) error {
	_, _ = ctx, name
	return ErrDataPlaneUnavailable
}

func (s *Service) DeleteBucket(ctx context.Context, name string) error {
	_, _ = ctx, name
	return ErrDataPlaneUnavailable
}

func (s *Service) ListObjects(ctx context.Context, bucket, prefix string) (*collectionlist.List[ObjectMeta], error) {
	_, _, _ = ctx, bucket, prefix
	return nil, ErrDataPlaneUnavailable
}

func (s *Service) PutObject(ctx context.Context, bucket, key string, reader io.Reader, opts PutOptions) (ObjectMeta, error) {
	_, _, _, _, _ = ctx, bucket, key, reader, opts
	return ObjectMeta{}, ErrDataPlaneUnavailable
}

func (s *Service) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectMeta, error) {
	_, _, _ = ctx, bucket, key
	return nil, ObjectMeta{}, ErrDataPlaneUnavailable
}

func (s *Service) StatObject(ctx context.Context, bucket, key string) (ObjectMeta, error) {
	_, _, _ = ctx, bucket, key
	return ObjectMeta{}, ErrDataPlaneUnavailable
}

func (s *Service) DeleteObject(ctx context.Context, bucket, key string) (ObjectMeta, error) {
	_, _, _ = ctx, bucket, key
	return ObjectMeta{}, ErrDataPlaneUnavailable
}

func (s *Service) CheckHealth(ctx context.Context, bucket, key string) (Health, error) {
	_, _, _ = ctx, bucket, key
	return Health{}, ErrDataPlaneUnavailable
}

func (s *Service) ScrubObject(ctx context.Context, bucket, key string) (ScrubResult, error) {
	_, _, _ = ctx, bucket, key
	return ScrubResult{}, ErrDataPlaneUnavailable
}

func (s *Service) RepairObject(ctx context.Context, bucket, key string) (RepairResult, error) {
	_, _, _ = ctx, bucket, key
	return RepairResult{}, ErrDataPlaneUnavailable
}

func (s *Service) PlanDedupe(ctx context.Context) (DedupeResult, error) {
	_ = ctx
	return DedupeResult{}, ErrDataPlaneUnavailable
}

func (s *Service) RunDedupe(ctx context.Context) (DedupeResult, error) {
	_ = ctx
	return DedupeResult{}, ErrDataPlaneUnavailable
}

func (s *Service) RebalanceNode(ctx context.Context, nodeID string) (RebalanceResult, error) {
	_, _ = ctx, nodeID
	return RebalanceResult{}, ErrDataPlaneUnavailable
}

func (s *Service) Recover(ctx context.Context) (RecoveryResult, error) {
	_ = ctx
	return RecoveryResult{}, ErrDataPlaneUnavailable
}

func (s *Service) RecoveryPlan(ctx context.Context) (RecoveryPlan, error) {
	_ = ctx
	return RecoveryPlan{}, ErrDataPlaneUnavailable
}

func (s *Service) RecoveryStatus() RecoveryStatus {
	return RecoveryStatus{}
}

func (s *Service) Search(ctx context.Context, query SearchQuery) (SearchResult, error) {
	_ = ctx
	if s == nil || s.search == nil {
		return SearchResult{}, ErrDataPlaneUnavailable
	}
	return s.search.Search(query), nil
}

func (s *Service) IndexStatus() IndexStatus {
	if s == nil {
		return IndexStatus{}
	}
	s.indexMu.RLock()
	defer s.indexMu.RUnlock()
	return s.index
}

func (s *Service) RebuildIndex(ctx context.Context) (IndexRebuildResult, error) {
	_ = ctx
	return IndexRebuildResult{}, ErrDataPlaneUnavailable
}

func (s *Service) StartIndexWorker(ctx context.Context) error {
	_ = ctx
	return nil
}

type ObjectEvent struct {
	Event   string
	Payload map[string]any
}

func (e ObjectEvent) Name() string {
	return e.Event
}
