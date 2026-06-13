package handler

import "net/http"

type repairHistoryResponse struct {
	Runs  []any `json:"runs"`
	Total int   `json:"total"`
}

type repairIssuesResponse struct {
	Issues []any `json:"issues"`
	Total  int   `json:"total"`
}

func (s *Service) handleRepairStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeLocalDataPlaneRemoved(w)
}

func (s *Service) handleRepairRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeLocalDataPlaneRemoved(w)
}

func (s *Service) handleRepairHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, repairHistoryResponse{})
}

func (s *Service) handleRepairIssues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeJSON(w, http.StatusOK, repairIssuesResponse{})
}
