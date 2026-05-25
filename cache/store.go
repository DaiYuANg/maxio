package cache

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/lyonbrown4d/maxio/object"
)

type Store struct {
	next   object.Store
	cache  MetadataCache
	logger *slog.Logger
}

func NewStore(next object.Store, cache MetadataCache, logger *slog.Logger) *Store {
	if logger == nil {
		logger = slog.Default()
	}
	if cache == nil {
		cache = NewNoopCache()
	}
	return &Store{
		next:   next,
		cache:  cache,
		logger: logger,
	}
}

func (s *Store) ListBuckets(ctx context.Context) ([]object.Bucket, error) {
	if buckets, ok, err := s.cache.GetBuckets(ctx); err != nil {
		s.logCacheError(ctx, "get buckets cache", err)
	} else if ok {
		return buckets, nil
	}
	buckets, err := s.next.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	if err := s.cache.SetBuckets(ctx, buckets); err != nil {
		s.logCacheError(ctx, "set buckets cache", err)
	}
	return buckets, nil
}

func (s *Store) CreateBucket(ctx context.Context, name string) error {
	if err := s.next.CreateBucket(ctx, name); err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	if err := s.cache.InvalidateAll(ctx); err != nil {
		s.logCacheError(ctx, "invalidate buckets cache", err)
	}
	return nil
}

func (s *Store) DeleteBucket(ctx context.Context, name string) error {
	if err := s.next.DeleteBucket(ctx, name); err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	if err := s.cache.InvalidateBucket(ctx, name); err != nil {
		s.logCacheError(ctx, "invalidate deleted bucket cache", err)
	}
	if err := s.cache.InvalidateAll(ctx); err != nil {
		s.logCacheError(ctx, "invalidate buckets cache", err)
	}
	return nil
}

func (s *Store) ListObjects(ctx context.Context, bucket, prefix string) ([]object.ObjectMeta, error) {
	if objects, ok, err := s.cache.GetListObjects(ctx, bucket, prefix); err != nil {
		s.logCacheError(ctx, "get object list cache", err)
	} else if ok {
		return objects, nil
	}
	objects, err := s.next.ListObjects(ctx, bucket, prefix)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	if err := s.cache.SetListObjects(ctx, bucket, prefix, objects); err != nil {
		s.logCacheError(ctx, "set object list cache", err)
	}
	return objects, nil
}

func (s *Store) PutObject(
	ctx context.Context,
	bucket string,
	key string,
	reader io.Reader,
	opts object.PutOptions,
) (object.ObjectMeta, error) {
	meta, err := s.next.PutObject(ctx, bucket, key, reader, opts)
	if err != nil {
		return object.ObjectMeta{}, fmt.Errorf("put object: %w", err)
	}
	if err := s.cache.SetObject(ctx, meta); err != nil {
		s.logCacheError(ctx, "set object cache", err)
	}
	if err := s.cache.InvalidateBucket(ctx, bucket); err != nil {
		s.logCacheError(ctx, "invalidate bucket cache", err)
	}
	return meta, nil
}

func (s *Store) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, object.ObjectMeta, error) {
	body, meta, err := s.next.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, object.ObjectMeta{}, fmt.Errorf("get object: %w", err)
	}
	if err := s.cache.SetObject(ctx, meta); err != nil {
		s.logCacheError(ctx, "set object cache", err)
	}
	return body, meta, nil
}

func (s *Store) StatObject(ctx context.Context, bucket, key string) (object.ObjectMeta, error) {
	if meta, ok, err := s.cache.GetObject(ctx, bucket, key); err != nil {
		s.logCacheError(ctx, "get object cache", err)
	} else if ok {
		return meta, nil
	}
	meta, err := s.next.StatObject(ctx, bucket, key)
	if err != nil {
		return object.ObjectMeta{}, fmt.Errorf("stat object: %w", err)
	}
	if err := s.cache.SetObject(ctx, meta); err != nil {
		s.logCacheError(ctx, "set object cache", err)
	}
	return meta, nil
}

func (s *Store) DeleteObject(ctx context.Context, bucket, key string) (object.ObjectMeta, error) {
	meta, err := s.next.DeleteObject(ctx, bucket, key)
	if err != nil {
		return object.ObjectMeta{}, fmt.Errorf("delete object: %w", err)
	}
	if err := s.cache.DeleteObject(ctx, bucket, key); err != nil {
		s.logCacheError(ctx, "delete object cache", err)
	}
	if err := s.cache.InvalidateBucket(ctx, bucket); err != nil {
		s.logCacheError(ctx, "invalidate bucket cache", err)
	}
	return meta, nil
}

func (s *Store) CheckHealth(ctx context.Context, bucket, key string) (object.Health, error) {
	result, err := s.next.CheckHealth(ctx, bucket, key)
	if err != nil {
		return object.Health{}, fmt.Errorf("check health: %w", err)
	}
	return result, nil
}

func (s *Store) ScrubObject(ctx context.Context, bucket, key string) (object.ScrubResult, error) {
	result, err := s.next.ScrubObject(ctx, bucket, key)
	if err != nil {
		return object.ScrubResult{}, fmt.Errorf("scrub object: %w", err)
	}
	return result, nil
}

func (s *Store) RepairObject(ctx context.Context, bucket, key string) (object.RepairResult, error) {
	result, err := s.next.RepairObject(ctx, bucket, key)
	if err != nil {
		return object.RepairResult{}, fmt.Errorf("repair object: %w", err)
	}
	return result, nil
}

func (s *Store) Dedupe(ctx context.Context, opts object.DedupeOptions) (object.DedupeResult, error) {
	result, err := s.next.Dedupe(ctx, opts)
	if err != nil {
		return result, fmt.Errorf("dedupe objects: %w", err)
	}
	if !opts.DryRun {
		if invalidateErr := s.cache.InvalidateAll(ctx); invalidateErr != nil {
			s.logCacheError(ctx, "invalidate cache after dedupe", invalidateErr)
		}
	}
	return result, nil
}

func (s *Store) RebalanceNode(ctx context.Context, nodeID string) (object.RebalanceResult, error) {
	result, err := s.next.RebalanceNode(ctx, nodeID)
	if err != nil {
		return result, fmt.Errorf("rebalance node: %w", err)
	}
	if err := s.cache.InvalidateAll(ctx); err != nil {
		s.logCacheError(ctx, "invalidate cache after rebalance", err)
	}
	return result, nil
}

func (s *Store) Recover(ctx context.Context, opts object.RecoveryOptions) (object.RecoveryResult, error) {
	result, err := s.next.Recover(ctx, opts)
	if err != nil {
		return result, fmt.Errorf("recover storage: %w", err)
	}
	if !opts.DryRun {
		if invalidateErr := s.cache.InvalidateAll(ctx); invalidateErr != nil {
			s.logCacheError(ctx, "invalidate cache after recovery", invalidateErr)
		}
	}
	return result, nil
}

func (s *Store) PlanRecovery(ctx context.Context, pendingTTL time.Duration) (object.RecoveryPlan, error) {
	result, err := s.next.PlanRecovery(ctx, pendingTTL)
	if err != nil {
		return object.RecoveryPlan{}, fmt.Errorf("plan recovery: %w", err)
	}
	return result, nil
}

func (s *Store) RecoveryStatus() object.RecoveryStatus {
	if s == nil || s.next == nil {
		return object.RecoveryStatus{}
	}
	return s.next.RecoveryStatus()
}

func (s *Store) logCacheError(ctx context.Context, op string, err error) {
	if s == nil || s.logger == nil || err == nil {
		return
	}
	s.logger.WarnContext(ctx, "metadata cache failed", "op", op, "error", err)
}
