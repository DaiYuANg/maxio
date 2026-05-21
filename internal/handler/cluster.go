package handler

import "net/http"

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
	if blockers := clusterMembershipChangeBlockers(membership, nodes); len(blockers) > 0 {
		s.writeClusterMembershipBlocked(w, blockers)
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
	if s.maybeHandleRemovedReplica(w, req.ReplicaID, req.Target, membership) {
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
	if s.maybeHandleRemovedReplica(w, req.ReplicaID, req.Target, membership) {
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
	membership, err := s.raft.GetMembership(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	if blockers := clusterMembershipChangeBlockers(membership, nodes); len(blockers) > 0 {
		s.writeClusterMembershipBlocked(w, blockers)
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
