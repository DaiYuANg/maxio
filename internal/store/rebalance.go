package store

import (
	"context"
	"fmt"

	"github.com/lyonbrown4d/maxio/model"
)

type RebalanceResult struct {
	NodeID  string `json:"node_id"`
	Objects int    `json:"objects"`
	Shards  int    `json:"shards"`
}

func (s *Store) RebalanceNode(ctx context.Context, nodeID string) (RebalanceResult, error) {
	if s == nil || s.engine == nil {
		return RebalanceResult{}, fmt.Errorf("%w: storage engine unavailable", ErrEngineFailed)
	}
	result := RebalanceResult{NodeID: nodeID}
	buckets, err := s.ListBuckets(ctx)
	if err != nil {
		return result, err
	}
	for _, bucket := range buckets {
		if err := s.rebalanceBucket(ctx, bucket.Name, nodeID, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (s *Store) rebalanceBucket(ctx context.Context, bucket, nodeID string, result *RebalanceResult) error {
	objects, err := s.ListObjects(ctx, bucket, "")
	if err != nil {
		return err
	}
	for index := range objects {
		if !metaUsesNode(objects[index], nodeID) {
			continue
		}
		if err := s.rebalanceObject(ctx, objects[index], nodeID, result); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) rebalanceObject(
	ctx context.Context,
	meta model.ObjectMeta,
	nodeID string,
	result *RebalanceResult,
) error {
	rebalance, err := s.engine.RebalanceObjectFromNode(ctx, meta.Bucket, meta.Key, nodeID)
	if err != nil {
		return fmt.Errorf("rebalance object %s/%s: %w", meta.Bucket, meta.Key, mapStoreError(err))
	}
	if len(rebalance.Moved) == 0 {
		return nil
	}
	meta.ShardPlacements = rebalance.Object.ShardPlacements
	if err := s.meta.UpsertObjectMeta(ctx, meta); err != nil {
		return mapStoreError(err)
	}
	if err := s.meta.UpdateBlobRefPlacements(ctx, meta.Hash, meta.ShardPlacements); err != nil {
		return mapStoreError(err)
	}
	result.Objects++
	result.Shards += len(rebalance.Moved)
	return nil
}

func metaUsesNode(meta model.ObjectMeta, nodeID string) bool {
	for _, placement := range meta.ShardPlacements {
		if placement.NodeID == nodeID {
			return true
		}
	}
	return false
}
