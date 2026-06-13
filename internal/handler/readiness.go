package handler

import (
	"context"
	"errors"
	"net/http"

	raftx "github.com/lyonbrown4d/maxio/internal/raft"
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
		err = joinReadiness(err, s.checkRaftReady(ctx, checks))
		err = joinReadiness(err, s.checkRaftLeaderReady(ctx, checks))
	} else {
		checks["raft_membership"] = "disabled"
		checks["raft_leader"] = "disabled"
	}
	s.checkRepairBacklogReady(checks)
	return err
}

func (s *Service) checkObjectServiceReady(checks map[string]string) error {
	if s.objects == nil {
		checks["object_service"] = "unavailable"
		return errReadinessUnavailable
	}
	checks["object_service"] = "ok"
	return nil
}

func (s *Service) checkGatewayDataPlaneReady(checks map[string]string) error {
	if !s.cfg.EnableNativeObjectAPI {
		if s.cfg.EnableS3Proxy {
			checks["s3_proxy"] = "configured"
			checks["object_service"] = "disabled"
			checks["engine"] = "removed"
			checks["storage_writable"] = "external_upstream"
			return nil
		}
		checks["s3_proxy"] = "not_implemented"
		checks["object_service"] = "disabled"
		checks["engine"] = "removed"
		checks["storage_writable"] = "external_upstream_pending"
		return nil
	}
	err := s.checkObjectServiceReady(checks)
	err = joinReadiness(err, s.checkEngineReady(checks))
	return joinReadiness(err, s.checkStorageWritableReady(checks))
}

func (s *Service) checkEngineReady(checks map[string]string) error {
	if s.engine == nil {
		checks["engine"] = "unavailable"
		return errReadinessUnavailable
	}
	if len(s.engine.StorageNodeInfos()) == 0 {
		checks["engine"] = "no_storage_nodes"
		return errReadinessUnavailable
	}
	checks["engine"] = "ok"
	return nil
}

func (s *Service) checkStorageWritableReady(checks map[string]string) error {
	if s.engine == nil {
		checks["storage_writable"] = "unavailable"
		return errReadinessUnavailable
	}
	nodes := s.engine.StorageNodeInfos()
	if len(nodes) == 0 {
		checks["storage_writable"] = "no_storage_nodes"
		return errReadinessUnavailable
	}
	for _, node := range nodes {
		if !node.Drained {
			checks["storage_writable"] = "ok"
			return nil
		}
	}
	checks["storage_writable"] = "no_writable_storage_nodes"
	return errReadinessUnavailable
}

func (s *Service) checkRaftReady(ctx context.Context, checks map[string]string) error {
	if s.raft == nil {
		checks["raft_membership"] = "unavailable"
		return errReadinessUnavailable
	}
	if _, err := s.raft.GetMembership(ctx); err != nil {
		checks["raft_membership"] = err.Error()
		return errReadinessUnavailable
	}
	checks["raft_membership"] = "ok"
	return nil
}

func (s *Service) checkRaftLeaderReady(ctx context.Context, checks map[string]string) error {
	if s.raft == nil {
		checks["raft_leader"] = "unavailable"
		return errReadinessUnavailable
	}
	err := s.raft.AssertLeader(ctx)
	if err == nil {
		checks["raft_leader"] = "local"
		return nil
	}
	if errors.Is(err, raftx.ErrNotLeader) {
		checks["raft_leader"] = "remote"
		return nil
	}
	checks["raft_leader"] = err.Error()
	return errReadinessUnavailable
}

func (s *Service) checkRepairBacklogReady(checks map[string]string) {
	if s.repair == nil {
		checks["repair_backlog"] = "disabled"
		return
	}
	status := s.repair.Status()
	switch {
	case status.Running:
		checks["repair_backlog"] = "running"
	case status.LastError != "":
		checks["repair_backlog"] = "last_run_failed"
	case status.LastSummary.Unrecoverable > 0:
		checks["repair_backlog"] = "unrecoverable"
	case status.LastSummary.Failed > 0:
		checks["repair_backlog"] = "failed"
	case status.LastSummary.Limited:
		checks["repair_backlog"] = "limited"
	default:
		checks["repair_backlog"] = "ok"
	}
}
