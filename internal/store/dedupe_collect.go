package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

func (s *Store) collectDedupeObjects(
	ctx context.Context,
	result *DedupeResult,
) ([]dedupeObject, map[string]dedupeHashStat, error) {
	buckets, err := s.meta.ListBuckets(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list buckets for dedupe: %w", mapStoreError(err))
	}
	result.Buckets = len(buckets)
	objects := make([]dedupeObject, 0)
	stats := make(map[string]dedupeHashStat)
	for index := range buckets {
		if err := s.collectBucketDedupeObjects(ctx, buckets[index].Name, &objects, stats, result); err != nil {
			return nil, nil, err
		}
	}
	result.Hashes = len(stats)
	return objects, stats, nil
}

func (s *Store) collectBucketDedupeObjects(
	ctx context.Context,
	bucket string,
	objects *[]dedupeObject,
	stats map[string]dedupeHashStat,
	result *DedupeResult,
) error {
	items, err := s.meta.ListObjectMetas(ctx, bucket, "")
	if err != nil {
		return fmt.Errorf("list bucket objects for dedupe: %w", mapStoreError(err))
	}
	for index := range items {
		object := s.loadDedupeObject(ctx, items[index])
		*objects = append(*objects, object)
		result.Objects++
		recordDedupeLayoutReadError(object, result)
		recordDedupeHashStat(object, stats)
	}
	return nil
}

func (s *Store) loadDedupeObject(ctx context.Context, meta model.ObjectMeta) dedupeObject {
	info, err := s.engine.StatObject(ctx, meta.Bucket, meta.Key)
	if err != nil {
		return dedupeObject{meta: meta, err: err}
	}
	return dedupeObject{meta: meta, info: info, hasInfo: true}
}

func recordDedupeLayoutReadError(object dedupeObject, result *DedupeResult) {
	if object.err == nil {
		return
	}
	result.addIssue(DedupeIssue{
		Kind:   DedupeIssueLayoutReadFailed,
		Bucket: object.meta.Bucket,
		Key:    object.meta.Key,
		Hash:   object.meta.Hash,
		Reason: object.err.Error(),
	})
}

func recordDedupeHashStat(object dedupeObject, stats map[string]dedupeHashStat) {
	hash := strings.TrimSpace(object.meta.Hash)
	if hash == "" {
		return
	}
	stat := stats[hash]
	stat.count++
	stat.size = object.meta.Size
	if stat.first.meta.Bucket == "" {
		stat.first = object
	}
	stats[hash] = stat
}
