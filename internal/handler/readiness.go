package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/lyonbrown4d/maxio/internal/control"
)

type readinessResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func (s *Service) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	response := s.readiness(r.Context())
	if response.Status != "ok" {
		s.writeJSON(w, http.StatusServiceUnavailable, response)
		return
	}
	s.writeJSON(w, http.StatusOK, response)
}

func (s *Service) readiness(ctx context.Context) readinessResponse {
	checks := map[string]string{}
	status := "ok"
	if err := s.checkReady(ctx, checks); err != nil {
		status = "not_ready"
	}
	return readinessResponse{Status: status, Checks: checks}
}

func (s *Service) checkReady(ctx context.Context, checks map[string]string) error {
	if s == nil {
		checks["service"] = "unavailable"
		return errReadinessUnavailable
	}
	checks["service"] = "ok"
	err := s.checkGatewayDataPlaneReady(checks)
	if s.cfg.EnableClusterManagement {
		err = joinReadiness(err, s.checkControlReady(ctx, checks))
		err = joinReadiness(err, s.checkControlLeaderReady(ctx, checks))
	} else {
		checks["control_membership"] = "disabled"
		checks["control_leader"] = "disabled"
	}
	return err
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

func (s *Service) checkControlReady(ctx context.Context, checks map[string]string) error {
	if s.control == nil {
		checks["control_membership"] = "unavailable"
		return errReadinessUnavailable
	}
	if _, err := s.control.GetMembership(ctx); err != nil {
		checks["control_membership"] = err.Error()
		return errReadinessUnavailable
	}
	checks["control_membership"] = "ok"
	return nil
}

func (s *Service) checkControlLeaderReady(ctx context.Context, checks map[string]string) error {
	if s.control == nil {
		checks["control_leader"] = "unavailable"
		return errReadinessUnavailable
	}
	err := s.control.AssertLeader(ctx)
	if err == nil {
		checks["control_leader"] = "local"
		return nil
	}
	if errors.Is(err, control.ErrNotLeader) {
		checks["control_leader"] = "remote"
		return nil
	}
	checks["control_leader"] = err.Error()
	return errReadinessUnavailable
}
