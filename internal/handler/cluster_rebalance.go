package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/lyonbrown4d/maxio/internal/model"
	raftx "github.com/lyonbrown4d/maxio/internal/raft"
)

type rebalancePlanResponse struct {
	ReplicaID uint64 `json:"replica_id"`
	NodeID    string `json:"node_id"`
	Objects   int    `json:"objects"`
	Shards    int    `json:"shards"`
	UsedBytes int64  `json:"used_bytes"`
}

type rebalanceResponse struct {
	ReplicaID uint64 `json:"replica_id"`
	NodeID    string `json:"node_id"`
	Objects   int    `json:"objects"`
	Shards    int    `json:"shards"`
	UsedBytes int64  `json:"used_bytes"`
	Status    string `json:"status"`
}

var errClusterRebalanceMemberNotFound = errors.New("cluster rebalance member not found")

func (s *Service) handleClusterRebalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	replicaID, err := parseRequiredReplicaID(r)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := s.rebalanceClusterMember(r.Context(), replicaID)
	if err != nil {
		s.writeClusterRebalanceError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.rebalance",
		"replica_id", replicaID,
		"node_id", result.NodeID,
		"objects", result.Objects,
		"shards", result.Shards,
		"used_bytes", result.UsedBytes,
		"status", result.Status,
	)
	s.writeJSON(w, http.StatusAccepted, result)
}

func (s *Service) handleClusterRebalancePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	replicaID, err := parseRequiredReplicaID(r)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	plan, err := s.planClusterRebalance(r.Context(), replicaID)
	if err != nil {
		s.writeClusterRebalanceError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, plan)
}

func (s *Service) writeClusterRebalanceError(w http.ResponseWriter, err error) {
	if errors.Is(err, errClusterRebalanceMemberNotFound) {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.writeError(w, err)
}

func (s *Service) rebalanceClusterMember(ctx context.Context, replicaID uint64) (rebalanceResponse, error) {
	if s == nil || s.objects == nil || s.raft == nil {
		return rebalanceResponse{}, errors.New("cluster rebalance dependencies unavailable")
	}
	nodeID, err := s.resolveClusterRebalanceNode(ctx, replicaID)
	if err != nil {
		return rebalanceResponse{}, err
	}
	stats, err := s.countObjectPlacements(ctx, nodeID)
	if err != nil {
		return rebalanceResponse{}, err
	}
	response := rebalanceResponse{
		ReplicaID: replicaID,
		NodeID:    nodeID,
		Objects:   stats.objects,
		Shards:    stats.shards,
		UsedBytes: stats.usedBytes,
	}
	if !stats.hasPlacements() {
		response.Status = "already_balanced"
		return response, nil
	}
	result, err := s.objects.RebalanceNode(ctx, nodeID)
	if err != nil {
		return rebalanceResponse{}, fmt.Errorf("rebalance cluster member: %w", err)
	}
	response.NodeID = result.NodeID
	response.Objects = result.Objects
	response.Shards = result.Shards
	response.Status = "rebalanced"
	return response, nil
}

func parseRequiredReplicaID(r *http.Request) (uint64, error) {
	value := r.URL.Query().Get("replica_id")
	if value == "" {
		return 0, errors.New("replica_id is required")
	}
	replicaID, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse replica_id: %w", err)
	}
	if replicaID == 0 {
		return 0, errors.New("replica_id must be greater than zero")
	}
	return replicaID, nil
}

func (s *Service) planClusterRebalance(ctx context.Context, replicaID uint64) (rebalancePlanResponse, error) {
	if s == nil || s.objects == nil || s.raft == nil {
		return rebalancePlanResponse{}, errors.New("cluster rebalance dependencies unavailable")
	}
	nodeID, err := s.resolveClusterRebalanceNode(ctx, replicaID)
	if err != nil {
		return rebalancePlanResponse{}, err
	}
	stats, err := s.countObjectPlacements(ctx, nodeID)
	if err != nil {
		return rebalancePlanResponse{}, err
	}
	return rebalancePlanResponse{
		ReplicaID: replicaID,
		NodeID:    nodeID,
		Objects:   stats.objects,
		Shards:    stats.shards,
		UsedBytes: stats.usedBytes,
	}, nil
}

func (s *Service) resolveClusterRebalanceNode(ctx context.Context, replicaID uint64) (string, error) {
	membership, err := s.raft.GetMembership(ctx)
	if err != nil {
		return "", fmt.Errorf("get raft membership: %w", err)
	}
	if err := ValidateClusterMemberRebalance(replicaID, membership); err != nil {
		return "", err
	}
	return clusterStorageNodeID(replicaID), nil
}

// ValidateClusterMemberRebalance validates whether a replica can be used as a rebalance source.
func ValidateClusterMemberRebalance(replicaID uint64, membership raftx.Membership) error {
	if replicaID == 0 {
		return errors.New("replica_id must be greater than zero")
	}
	if _, ok := membership.Nodes[replicaID]; !ok {
		return fmt.Errorf("%w: replica %d", errClusterRebalanceMemberNotFound, replicaID)
	}
	return nil
}

type nodePlacementStats struct {
	objects   int
	shards    int
	usedBytes int64
}

func (s nodePlacementStats) hasPlacements() bool {
	return s.objects > 0 || s.shards > 0 || s.usedBytes > 0
}

func (s *Service) countObjectPlacements(ctx context.Context, nodeID string) (nodePlacementStats, error) {
	buckets, err := s.objects.ListBuckets(ctx)
	if err != nil {
		return nodePlacementStats{}, fmt.Errorf("list buckets for rebalance plan: %w", err)
	}
	var stats nodePlacementStats
	for _, bucket := range buckets {
		entries, err := s.objects.ListObjects(ctx, bucket.Name, "")
		if err != nil {
			return nodePlacementStats{}, fmt.Errorf("list objects for rebalance plan: %w", err)
		}
		stats.add(countObjectPlacements(entries, nodeID))
	}
	return stats, nil
}

func (s *nodePlacementStats) add(other nodePlacementStats) {
	s.objects += other.objects
	s.shards += other.shards
	s.usedBytes += other.usedBytes
}

func countObjectPlacements(objects []model.ObjectMeta, nodeID string) nodePlacementStats {
	var stats nodePlacementStats
	for index := range objects {
		object := &objects[index]
		matched := false
		for _, placement := range object.ShardPlacements {
			if placement.NodeID != nodeID {
				continue
			}
			matched = true
			stats.shards++
			stats.usedBytes += objectShardSize(object, placement.Index)
		}
		if matched {
			stats.objects++
		}
	}
	return stats
}

func objectShardSize(meta *model.ObjectMeta, shardIndex int) int64 {
	if meta == nil {
		return 0
	}
	if shardIndex < 0 {
		return 0
	}
	for index, size := range meta.ShardSizes {
		if index == shardIndex {
			return size
		}
	}
	return 0
}
