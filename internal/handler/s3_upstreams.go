package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const defaultS3UpstreamsPath = "/_s3/upstreams"

type upstreamRequest struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Endpoint string   `json:"endpoint"`
	Region   string   `json:"region"`
	Weight   int      `json:"weight"`
	Priority int      `json:"priority"`
	Buckets  []string `json:"buckets"`
	Enabled  *bool    `json:"enabled"`
}

func (s *Service) handleS3Upstreams(w http.ResponseWriter, r *http.Request) {
	if s.metadata == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "metadata repository is unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.listS3Upstreams(w, r)
	case http.MethodPost:
		s.upsertS3Upstream(w, r, "")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleS3Upstream(w http.ResponseWriter, r *http.Request, id string) {
	if s.metadata == nil {
		s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "metadata repository is unavailable"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.getS3Upstream(w, r, id)
	case http.MethodPut:
		s.upsertS3Upstream(w, r, id)
	case http.MethodDelete:
		s.deleteS3Upstream(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) listS3Upstreams(w http.ResponseWriter, r *http.Request) {
	upstreams, err := s.metadata.ListUpstreams(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, upstreams)
}

func (s *Service) getS3Upstream(w http.ResponseWriter, r *http.Request, id string) {
	upstream, ok, err := s.metadata.GetUpstream(r.Context(), id)
	if err != nil {
		s.writeS3UpstreamError(w, err)
		return
	}
	if !ok {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "upstream not found"})
		return
	}
	s.writeJSON(w, http.StatusOK, upstream)
}

func (s *Service) upsertS3Upstream(w http.ResponseWriter, r *http.Request, id string) {
	upstream, err := decodeUpstreamRequest(r, id)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	stored, err := s.metadata.UpsertUpstream(r.Context(), upstream)
	if err != nil {
		s.writeS3UpstreamError(w, err)
		return
	}
	s.auditHTTP(r, "s3.upstream.upsert", "upstream_id", stored.ID, "endpoint", stored.Endpoint)
	s.writeJSON(w, http.StatusOK, stored)
}

func (s *Service) deleteS3Upstream(w http.ResponseWriter, r *http.Request, id string) {
	deleted, err := s.metadata.DeleteUpstream(r.Context(), id)
	if err != nil {
		s.writeS3UpstreamError(w, err)
		return
	}
	if !deleted {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "upstream not found"})
		return
	}
	s.auditHTTP(r, "s3.upstream.delete", "upstream_id", strings.TrimSpace(id))
	w.WriteHeader(http.StatusNoContent)
}

func decodeUpstreamRequest(r *http.Request, pathID string) (model.Upstream, error) {
	var payload upstreamRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return model.Upstream{}, fmt.Errorf("decode upstream request: %w", err)
	}
	if strings.TrimSpace(pathID) != "" {
		payload.ID = pathID
	}
	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}
	upstream := model.Upstream{
		ID:       payload.ID,
		Name:     payload.Name,
		Endpoint: payload.Endpoint,
		Region:   payload.Region,
		Weight:   payload.Weight,
		Priority: payload.Priority,
		Buckets:  payload.Buckets,
		Enabled:  enabled,
	}
	if upstream.Weight == 0 {
		upstream.Weight = 1
	}
	return upstream, nil
}

func (s *Service) writeS3UpstreamError(w http.ResponseWriter, err error) {
	if errors.Is(err, metadata.ErrBadRequest) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.writeError(w, err)
}
