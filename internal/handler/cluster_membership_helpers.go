package handler

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/discovery"
	raftx "github.com/lyonbrown4d/maxio/internal/raft"
)

const (
	clusterMembershipStatusBlocked                  = "blocked"
	clusterMembershipReasonAddressChangeBlocked     = "address_change_blocked"
	clusterMembershipReasonRemovedReplicaReappeared = "removed_replica_reappeared"
)

type clusterMembershipBlockedResponse struct {
	Error     string                           `json:"error"`
	Status    string                           `json:"status"`
	Reason    string                           `json:"reason,omitempty"`
	ReplicaID uint64                           `json:"replica_id,omitempty"`
	Current   string                           `json:"current,omitempty"`
	Requested string                           `json:"requested,omitempty"`
	Blocked   []clusterMembershipBlockedChange `json:"blocked,omitempty"`
}

type clusterMembershipBlockedChange struct {
	Reason    string `json:"reason"`
	ReplicaID uint64 `json:"replica_id"`
	Current   string `json:"current,omitempty"`
	Requested string `json:"requested,omitempty"`
}

func membershipStatesMatch(current, desired map[uint64]string) bool {
	if len(current) != len(desired) {
		return false
	}
	for replicaID, target := range current {
		if desired[replicaID] != target {
			return false
		}
	}
	return true
}

func (s *Service) maybeHandleExistingReplica(
	w http.ResponseWriter,
	r *http.Request,
	replicaID uint64,
	target string,
	nodes map[uint64]string,
	status string,
) bool {
	currentTarget, exists := nodes[replicaID]
	if !exists {
		return false
	}
	if currentTarget == target {
		if err := s.syncStorageNodes(r.Context()); err != nil {
			s.writeError(w, err)
			return true
		}
		s.writeJSON(w, http.StatusOK, map[string]any{
			"replica_id": replicaID,
			"target":     target,
			"status":     status,
		})
		return true
	}
	s.writeClusterMembershipBlocked(w, []clusterMembershipBlockedChange{{
		Reason:    clusterMembershipReasonAddressChangeBlocked,
		ReplicaID: replicaID,
		Current:   currentTarget,
		Requested: target,
	}})
	return true
}

func (s *Service) maybeHandleRemovedReplica(
	w http.ResponseWriter,
	replicaID uint64,
	target string,
	membership raftx.Membership,
) bool {
	if !isRemovedReplica(membership.Removed, replicaID) {
		return false
	}
	s.writeClusterMembershipBlocked(w, []clusterMembershipBlockedChange{{
		Reason:    clusterMembershipReasonRemovedReplicaReappeared,
		ReplicaID: replicaID,
		Requested: target,
	}})
	return true
}

func (s *Service) writeClusterMembershipBlocked(w http.ResponseWriter, blockers []clusterMembershipBlockedChange) {
	response := clusterMembershipBlockedResponse{
		Error:   clusterMembershipBlockedError(blockers),
		Status:  clusterMembershipStatusBlocked,
		Blocked: blockers,
	}
	if len(blockers) == 1 {
		response.Reason = blockers[0].Reason
		response.ReplicaID = blockers[0].ReplicaID
		response.Current = blockers[0].Current
		response.Requested = blockers[0].Requested
		response.Blocked = nil
	}
	s.writeJSON(w, http.StatusConflict, response)
}

func clusterMembershipChangeBlockers(
	membership raftx.Membership,
	desired map[uint64]string,
) []clusterMembershipBlockedChange {
	blockers := make([]clusterMembershipBlockedChange, 0)
	for replicaID, requested := range desired {
		if current, ok := membership.Nodes[replicaID]; ok && current != requested {
			blockers = append(blockers, clusterMembershipBlockedChange{
				Reason:    clusterMembershipReasonAddressChangeBlocked,
				ReplicaID: replicaID,
				Current:   current,
				Requested: requested,
			})
			continue
		}
		if isRemovedReplica(membership.Removed, replicaID) {
			blockers = append(blockers, clusterMembershipBlockedChange{
				Reason:    clusterMembershipReasonRemovedReplicaReappeared,
				ReplicaID: replicaID,
				Requested: requested,
			})
		}
	}
	slices.SortFunc(blockers, compareClusterMembershipBlockedChanges)
	return blockers
}

func compareClusterMembershipBlockedChanges(left, right clusterMembershipBlockedChange) int {
	if byReplica := cmp.Compare(left.ReplicaID, right.ReplicaID); byReplica != 0 {
		return byReplica
	}
	return cmp.Compare(left.Reason, right.Reason)
}

func clusterMembershipBlockedError(blockers []clusterMembershipBlockedChange) string {
	if len(blockers) != 1 {
		return "cluster membership change blocked"
	}
	blocker := blockers[0]
	switch blocker.Reason {
	case clusterMembershipReasonAddressChangeBlocked:
		return fmt.Sprintf("raft replica %d already exists with different target", blocker.ReplicaID)
	case clusterMembershipReasonRemovedReplicaReappeared:
		return fmt.Sprintf("raft replica %d has been removed and cannot be added back", blocker.ReplicaID)
	default:
		return "cluster membership change blocked"
	}
}

func isRemovedReplica(removed []uint64, replicaID uint64) bool {
	for _, removedReplicaID := range removed {
		if removedReplicaID == replicaID {
			return true
		}
	}
	return false
}

func removedReplicaSet(removed []uint64) map[uint64]struct{} {
	replicas := make(map[uint64]struct{}, len(removed))
	for _, replicaID := range removed {
		replicas[replicaID] = struct{}{}
	}
	return replicas
}

func (s *Service) syncStorageNodes(ctx context.Context) error {
	if s == nil || s.engine == nil || s.raft == nil {
		return nil
	}

	membership, err := s.raft.GetMembership(ctx)
	if err != nil {
		return fmt.Errorf("get raft membership: %w", err)
	}
	localReplicaID := s.raft.LocalReplicaID()
	if localReplicaID == 0 {
		return errors.New("local raft replica id is missing")
	}
	s.engine.SetControlToken(s.storageNodeToken())
	storageNodes := s.storageNodesFromMembership(membership.Nodes)
	if err := s.engine.SyncStorageNodesFromRaft(localReplicaID, storageNodes); err != nil {
		return fmt.Errorf("sync engine storage nodes: %w", err)
	}
	return nil
}

func (s *Service) storageNodesFromMembership(raftNodes map[uint64]string) map[uint64]string {
	storageNodes := make(map[uint64]string, len(raftNodes))
	maps.Copy(storageNodes, raftNodes)
	for _, node := range s.discoveryNodes() {
		if node.ReplicaID == 0 || strings.TrimSpace(node.HTTPAddress) == "" {
			continue
		}
		if _, ok := storageNodes[node.ReplicaID]; ok {
			storageNodes[node.ReplicaID] = strings.TrimSpace(node.HTTPAddress)
		}
	}
	return storageNodes
}

func (s *Service) discoveryNodes() []discovery.Node {
	if s == nil || s.discovery == nil {
		return nil
	}
	return s.discovery.Nodes()
}
