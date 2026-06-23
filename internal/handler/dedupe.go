package handler

import "net/http"

func (s *Service) handleDedupeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dedupe == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dedupe runtime unavailable"})
		return
	}
	s.writeJSON(w, http.StatusOK, s.dedupe.Status())
}

func (s *Service) handleDedupePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dedupe == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dedupe runtime unavailable"})
		return
	}
	result, err := s.dedupe.PlanOnce(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}

func (s *Service) handleDedupeRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.dedupe == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dedupe runtime unavailable"})
		return
	}
	result, err := s.dedupe.RunOnce(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "dedupe.run", "objects", result.Objects, "fixes", result.Fixes, "limited", result.Limited)
	s.writeJSON(w, http.StatusAccepted, result)
}
