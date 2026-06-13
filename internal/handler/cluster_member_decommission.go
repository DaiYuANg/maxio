package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/lyonbrown4d/maxio/internal/control"
)

var (
	errCannotDecommissionLocalReplica = errors.New("cannot decommission local replica")
	errClusterDecommissionBlocked     = errors.New("cluster member decommission blocked")
)

type clusterMemberDecommissionResponse struct {
	ReplicaID uint64 `json:"replica_id"`
	NodeID    string `json:"node_id"`
	Objects   int    `json:"objects"`
	Shards    int    `json:"shards"`
	UsedBytes int64  `json:"used_bytes"`
	Status    string `json:"status"`
}

type clusterMemberDecommissionBlockedResponse struct {
	Error     string `json:"error"`
	ReplicaID uint64 `json:"replica_id"`
	NodeID    string `json:"node_id"`
	Objects   int    `json:"objects"`
	Shards    int    `json:"shards"`
	UsedBytes int64  `json:"used_bytes"`
	Status    string `json:"status"`
}

func (s *Service) handleDecommissionClusterMember(w http.ResponseWriter, r *http.Request, replicaID uint64) {
	result, err := s.decommissionClusterMember(r.Context(), replicaID)
	if err != nil {
		s.writeDecommissionError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.member.decommission", "replica_id", replicaID, "node_id", result.NodeID, "status", result.Status)
	s.writeJSON(w, http.StatusAccepted, result)
}

func (s *Service) writeDecommissionError(w http.ResponseWriter, err error) {
	if errors.Is(err, errCannotDecommissionLocalReplica) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if response, ok := decommissionBlockedResponse(err); ok {
		s.writeJSON(w, http.StatusConflict, response)
		return
	}
	s.writeError(w, err)
}

func decommissionBlockedResponse(err error) (clusterMemberDecommissionBlockedResponse, bool) {
	var blocked *clusterDecommissionBlockedError
	if !errors.As(err, &blocked) {
		return clusterMemberDecommissionBlockedResponse{}, false
	}
	return clusterMemberDecommissionBlockedResponse{
		Error:     blocked.Error(),
		ReplicaID: blocked.replicaID,
		NodeID:    blocked.nodeID,
		Objects:   blocked.stats.objects,
		Shards:    blocked.stats.shards,
		UsedBytes: blocked.stats.usedBytes,
		Status:    "blocked",
	}, true
}

func (s *Service) decommissionClusterMember(ctx context.Context, replicaID uint64) (clusterMemberDecommissionResponse, error) {
	if s == nil || s.control == nil {
		return clusterMemberDecommissionResponse{}, errors.New("cluster decommission dependencies unavailable")
	}
	membership, err := s.control.GetMembership(ctx)
	if err != nil {
		return clusterMemberDecommissionResponse{}, fmt.Errorf("get control membership: %w", err)
	}
	present, err := ValidateClusterMemberDecommission(replicaID, membership)
	if err != nil {
		return clusterMemberDecommissionResponse{}, err
	}
	nodeID := clusterStorageNodeID(replicaID)
	if !present {
		return clusterMemberDecommissionResponse{ReplicaID: replicaID, NodeID: nodeID, Status: "already_decommissioned"}, nil
	}
	if err := s.control.RemoveReplica(ctx, replicaID); err != nil {
		return clusterMemberDecommissionResponse{}, fmt.Errorf("remove decommissioned cluster replica: %w", err)
	}
	if err := s.syncStorageNodes(ctx); err != nil {
		return clusterMemberDecommissionResponse{}, fmt.Errorf("sync storage nodes after decommission: %w", err)
	}
	return clusterMemberDecommissionResponse{ReplicaID: replicaID, NodeID: nodeID, Status: "decommissioned"}, nil
}

func ValidateClusterMemberDecommission(replicaID uint64, membership control.Membership) (bool, error) {
	if replicaID == 0 {
		return false, errors.New("replica_id must be greater than zero")
	}
	if replicaID == membership.LocalReplicaID {
		return false, errCannotDecommissionLocalReplica
	}
	_, present := membership.Nodes[replicaID]
	return present, nil
}
