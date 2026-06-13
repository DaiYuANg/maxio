package handler

import "net/http"

func (s *Service) handleClusterStorageNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, []StorageNodeInfo{})
}

func (s *Service) handleClusterStorageNodesSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.syncStorageNodes(r.Context()); err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.storage_nodes.sync")
	s.writeJSON(w, http.StatusOK, []StorageNodeInfo{})
}
