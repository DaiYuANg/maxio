package handler

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/discovery"
	"github.com/lyonbrown4d/maxio/internal/engine"
	raftx "github.com/lyonbrown4d/maxio/internal/raft"
)

type clusterStatusResponse struct {
	LocalReplicaID  uint64                   `json:"local_replica_id"`
	LocalRaftTarget string                   `json:"local_raft_target"`
	Membership      raftx.Membership         `json:"membership"`
	Nodes           []ClusterNodeInfo        `json:"nodes"`
	StorageNodes    []engine.StorageNodeInfo `json:"storage_nodes"`
	DiscoveryNodes  []discovery.Node         `json:"discovery_nodes"`
	Reconcile       clusterReconcilePlan     `json:"reconcile"`
}

type clusterReconcileRequest struct {
	RemoveMissing bool `json:"remove_missing"`
}

type clusterReconcileResponse struct {
	Plan   clusterReconcilePlan       `json:"plan"`
	Result raftx.SyncMembershipResult `json:"result"`
	Status string                     `json:"status"`
}

type clusterReconcilePlan struct {
	Mode      string                     `json:"mode"`
	Current   map[uint64]string          `json:"current"`
	Desired   map[uint64]string          `json:"desired"`
	Added     []raftx.Replica            `json:"added,omitempty"`
	Removed   []uint64                   `json:"removed,omitempty"`
	Conflicts []clusterDiscoveryConflict `json:"conflicts,omitempty"`
}

type clusterDiscoveryConflict struct {
	ReplicaID  uint64 `json:"replica_id"`
	Reason     string `json:"reason"`
	Current    string `json:"current"`
	Discovered string `json:"discovered"`
}

func (s *Service) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	membership, err := s.clusterMembership(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	plan := BuildClusterReconcilePlan(membership, s.discoveryNodes(), false)
	storageNodes := s.clusterStorageNodeInfos()
	s.writeJSON(w, http.StatusOK, clusterStatusResponse{
		LocalReplicaID:  membership.LocalReplicaID,
		LocalRaftTarget: s.localRaftTarget(),
		Membership:      membership,
		Nodes:           BuildClusterNodeRegistry(membership, s.discoveryNodes(), storageNodes),
		StorageNodes:    storageNodes,
		DiscoveryNodes:  s.discoveryNodes(),
		Reconcile:       plan,
	})
}

func (s *Service) handleClusterReconcile(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleClusterReconcilePlan(w, r)
	case http.MethodPost:
		s.handleClusterReconcileApply(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleClusterReconcilePlan(w http.ResponseWriter, r *http.Request) {
	removeMissing, err := parseBoolQuery(r, "remove_missing")
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	plan, err := s.clusterReconcilePlan(r.Context(), removeMissing)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, plan)
}

func (s *Service) handleClusterReconcileApply(w http.ResponseWriter, r *http.Request) {
	req := clusterReconcileRequest{}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	plan, err := s.clusterReconcilePlan(r.Context(), req.RemoveMissing)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if len(plan.Conflicts) > 0 {
		s.writeJSON(w, http.StatusConflict, clusterReconcileResponse{Plan: plan, Status: "conflict"})
		return
	}
	result, err := s.raft.SyncReplicas(r.Context(), plan.Desired)
	if err != nil {
		s.writeError(w, err)
		return
	}
	if err := s.syncStorageNodes(r.Context()); err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.reconcile", "mode", plan.Mode, "added", len(plan.Added), "removed", len(plan.Removed))
	s.writeJSON(w, http.StatusAccepted, clusterReconcileResponse{
		Plan:   plan,
		Result: result,
		Status: "applied",
	})
}

func (s *Service) clusterReconcilePlan(ctx context.Context, removeMissing bool) (clusterReconcilePlan, error) {
	membership, err := s.clusterMembership(ctx)
	if err != nil {
		return clusterReconcilePlan{}, err
	}
	return BuildClusterReconcilePlan(membership, s.discoveryNodes(), removeMissing), nil
}

func (s *Service) clusterMembership(ctx context.Context) (raftx.Membership, error) {
	if s == nil || s.raft == nil {
		return raftx.Membership{}, errors.New("raft runtime unavailable")
	}
	membership, err := s.raft.GetMembership(ctx)
	if err != nil {
		return raftx.Membership{}, fmt.Errorf("get raft membership: %w", err)
	}
	return membership, nil
}

func (s *Service) localRaftTarget() string {
	if s == nil || s.raft == nil {
		return ""
	}
	return s.raft.LocalRaftAddress()
}

func (s *Service) clusterStorageNodeInfos() []engine.StorageNodeInfo {
	if s == nil || s.engine == nil {
		return nil
	}
	return s.engine.StorageNodeInfos()
}

// BuildClusterReconcilePlan builds a safe raft membership reconcile plan from gossip discovery.
func BuildClusterReconcilePlan(
	membership raftx.Membership,
	discovered []discovery.Node,
	removeMissing bool,
) clusterReconcilePlan {
	desired := desiredMembersBase(membership, removeMissing)
	conflicts := mergeDiscoveredMembers(desired, membership.Nodes, removedReplicaSet(membership.Removed), discovered)
	if removeMissing {
		keepLocalMember(membership, desired)
	}
	return clusterReconcilePlan{
		Mode:      reconcileMode(removeMissing),
		Current:   maps.Clone(membership.Nodes),
		Desired:   desired,
		Added:     reconcileAdded(membership.Nodes, desired),
		Removed:   reconcileRemoved(membership.Nodes, desired),
		Conflicts: conflicts,
	}
}

func desiredMembersBase(membership raftx.Membership, removeMissing bool) map[uint64]string {
	if removeMissing {
		return make(map[uint64]string)
	}
	return maps.Clone(membership.Nodes)
}

func usableDiscoveredNode(node discovery.Node) bool {
	return node.ReplicaID > 0 && strings.EqualFold(node.State, "alive") && strings.TrimSpace(node.RaftAddress) != ""
}

func keepLocalMember(membership raftx.Membership, desired map[uint64]string) {
	if membership.LocalReplicaID == 0 {
		return
	}
	if _, ok := desired[membership.LocalReplicaID]; ok {
		return
	}
	if target := strings.TrimSpace(membership.Nodes[membership.LocalReplicaID]); target != "" {
		desired[membership.LocalReplicaID] = target
	}
}

func reconcileMode(removeMissing bool) string {
	if removeMissing {
		return "exact"
	}
	return "add_only"
}

func reconcileAdded(current, desired map[uint64]string) []raftx.Replica {
	replicas := make([]raftx.Replica, 0)
	for replicaID, target := range desired {
		if _, ok := current[replicaID]; !ok {
			replicas = append(replicas, raftx.Replica{ReplicaID: replicaID, Target: target})
		}
	}
	slices.SortFunc(replicas, func(left, right raftx.Replica) int {
		return cmp.Compare(left.ReplicaID, right.ReplicaID)
	})
	return replicas
}

func reconcileRemoved(current, desired map[uint64]string) []uint64 {
	replicaIDs := make([]uint64, 0)
	for replicaID := range current {
		if _, ok := desired[replicaID]; !ok {
			replicaIDs = append(replicaIDs, replicaID)
		}
	}
	slices.Sort(replicaIDs)
	return replicaIDs
}

func parseBoolQuery(r *http.Request, name string) (bool, error) {
	value := strings.TrimSpace(r.URL.Query().Get(name))
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", name, err)
	}
	return parsed, nil
}
