package handler

import (
	"errors"
	"net/http"

	"github.com/lyonbrown4d/maxio/internal/indexcontrol"
)

func (s *Service) handleIndexStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.indexManager == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "index manager is unavailable"})
		return
	}
	status, err := s.indexManager.Status(r.Context())
	if err != nil {
		s.writeIndexManagerError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, status)
}

func (s *Service) handleIndexRebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.indexManager == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "index manager is unavailable"})
		return
	}
	result, err := s.indexManager.Rebuild(r.Context())
	if err != nil {
		s.writeIndexManagerError(w, err)
		return
	}
	s.auditHTTP(r, "index.rebuild", "objects", result.Objects, "failed", result.Failed)
	s.writeJSON(w, http.StatusAccepted, result)
}

func (s *Service) writeIndexManagerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, indexcontrol.ErrManagerUnavailable):
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "index manager is unavailable"})
	case errors.Is(err, indexcontrol.ErrRebuildInProgress):
		s.writeJSON(w, http.StatusConflict, map[string]string{"error": "index rebuild already running"})
	default:
		s.writeError(w, err)
	}
}
