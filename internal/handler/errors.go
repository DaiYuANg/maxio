package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/lyonbrown4d/maxio/internal/object"
)

func (s *Service) writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if value == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(value); err != nil {
		s.logger.Warn("encode response body failed", slog.Any("error", err))
	}
}

func (s *Service) writeError(w http.ResponseWriter, err error) {
	msg := err.Error()
	if errors.Is(err, object.ErrBucketExists) {
		s.writeJSON(w, http.StatusConflict, map[string]string{"error": msg})
		return
	}
	if errors.Is(err, object.ErrBucketNotFound) || errors.Is(err, object.ErrNotFound) {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": msg})
		return
	}
	if errors.Is(err, object.ErrObjectCorrupted) {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": msg})
		return
	}
	s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg})
}
