package handler

import (
	"errors"
	"net/http"
)

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

var errReadinessUnavailable = errors.New("readiness unavailable")

func (s *Service) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	response := s.readiness()
	if response.Status != "ok" {
		s.writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Service) readiness() readinessResponse {
	checks := map[string]string{}
	status := "ok"
	if err := s.checkReady(checks); err != nil {
		status = "not_ready"
	}
	return readinessResponse{Status: status, Checks: checks}
}

func (s *Service) checkReady(checks map[string]string) error {
	if s == nil {
		checks["service"] = "unavailable"
		return errReadinessUnavailable
	}
	checks["service"] = "ok"
	return s.checkGatewayDataPlaneReady(checks)
}

func (s *Service) checkGatewayDataPlaneReady(checks map[string]string) error {
	checks["engine"] = "removed"
	checks["storage_writable"] = "external_upstream"
	if s.cfg.EnableS3Proxy {
		checks["s3_proxy"] = "configured"
		checks["object_service"] = "disabled"
		return nil
	}
	if s.cfg.EnableNativeObjectAPI {
		checks["s3_proxy"] = "disabled"
		checks["object_service"] = "removed"
		return errReadinessUnavailable
	}
	checks["s3_proxy"] = "not_implemented"
	checks["object_service"] = "disabled"
	return nil
}
