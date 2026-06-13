package handler

import "net/http"

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

type nodePlacementStats struct {
	objects   int
	shards    int
	usedBytes int64
}

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
	nodeID := clusterStorageNodeID(replicaID)
	s.writeJSON(w, http.StatusNotImplemented, rebalanceResponse{ReplicaID: replicaID, NodeID: nodeID, Status: "not_applicable"})
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
	s.writeJSON(w, http.StatusOK, rebalancePlanResponse{ReplicaID: replicaID, NodeID: clusterStorageNodeID(replicaID)})
}
