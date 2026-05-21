package handler

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/discovery"
)

type addReplicaRequest struct {
	ReplicaID uint64 `json:"replica_id"`
	Target    string `json:"target"`
}

func (s *Service) handleClusterMembers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListClusterMembers(w, r)
	case http.MethodPost:
		s.handleAddClusterMember(w, r)
	case http.MethodPut:
		s.handleSyncClusterMembers(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleClusterBootstrap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	nodes, err := decodeClusterNodeMap(r)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	membership, err := s.raft.GetMembership(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	if membershipStatesMatch(membership.Nodes, nodes) {
		err = s.syncStorageNodes(r.Context())
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, http.StatusOK, map[string]any{
			"status":  "already_bootstrapped",
			"members": len(nodes),
		})
		return
	}
	result, err := s.raft.SyncReplicas(r.Context(), nodes)
	if err != nil {
		s.writeError(w, err)
		return
	}
	err = s.syncStorageNodes(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.bootstrap", "members", len(nodes))
	s.writeJSON(w, http.StatusOK, result)
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
	s.writeJSON(w, http.StatusConflict, map[string]string{
		"error": fmt.Sprintf("raft replica %d already exists with different target", replicaID),
	})
	return true
}

func (s *Service) handleClusterJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req, err := decodeAddReplicaRequest(r, "join")
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	membership, err := s.raft.GetMembership(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	if s.maybeHandleExistingReplica(w, r, req.ReplicaID, req.Target, membership.Nodes, "already_joined") {
		return
	}
	err = s.raft.AddReplica(r.Context(), req.ReplicaID, req.Target)
	if err != nil {
		s.writeError(w, err)
		return
	}
	err = s.syncStorageNodes(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.member.join", "replica_id", req.ReplicaID, "target", req.Target)
	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"replica_id": req.ReplicaID,
		"target":     req.Target,
		"status":     "joined",
	})
}

func (s *Service) handleListClusterMembers(w http.ResponseWriter, r *http.Request) {
	membership, err := s.raft.GetMembership(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, membership)
}

func (s *Service) handleAddClusterMember(w http.ResponseWriter, r *http.Request) {
	req, err := decodeAddReplicaRequest(r, "add")
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	membership, err := s.raft.GetMembership(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	if s.maybeHandleExistingReplica(w, r, req.ReplicaID, req.Target, membership.Nodes, "already_added") {
		return
	}
	err = s.raft.AddReplica(r.Context(), req.ReplicaID, req.Target)
	if err != nil {
		s.writeError(w, err)
		return
	}
	err = s.syncStorageNodes(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.member.add", "replica_id", req.ReplicaID, "target", req.Target)
	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"replica_id": req.ReplicaID,
		"target":     req.Target,
		"status":     "added",
	})
}

func (s *Service) handleSyncClusterMembers(w http.ResponseWriter, r *http.Request) {
	nodes, err := decodeClusterNodeMap(r)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := s.raft.SyncReplicas(r.Context(), nodes)
	if err != nil {
		s.writeError(w, err)
		return
	}
	err = s.syncStorageNodes(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.members.sync", "members", len(nodes))
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Service) handleClusterMember(w http.ResponseWriter, r *http.Request, replicaID string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := parseReplicaIDSegment(replicaID)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	membership, err := s.raft.GetMembership(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	if id == membership.LocalReplicaID {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "cannot remove local replica",
		})
		return
	}
	if _, ok := membership.Nodes[id]; !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	err = s.ensureClusterMemberDecommissionable(r.Context(), id)
	if err != nil {
		s.writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	err = s.raft.RemoveReplica(r.Context(), id)
	if err != nil {
		s.writeError(w, err)
		return
	}
	err = s.syncStorageNodes(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.member.delete", "replica_id", id)
	w.WriteHeader(http.StatusNoContent)
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
