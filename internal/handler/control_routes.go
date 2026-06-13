package handler

import (
	"net/http"
	"strings"
)

func (s *Service) handleControlRoute(w http.ResponseWriter, r *http.Request, route string, parts []string) bool {
	switch {
	case isHealthRoute(route):
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return true
	case isReadinessRoute(route):
		s.handleReadiness(w, r)
		return true
	case isMetricsRoute(route):
		s.handleMetrics(w, r)
		return true
	}
	if isClusterRoute(route) && !s.cfg.EnableClusterManagement {
		http.NotFound(w, r)
		return true
	}
	return s.handleNamedControlRoute(w, r, route, parts)
}

func isClusterRoute(route string) bool {
	return strings.HasPrefix(strings.TrimSpace(route), "_cluster")
}

func (s *Service) handleNamedControlRoute(w http.ResponseWriter, r *http.Request, route string, parts []string) bool {
	routes := map[string]func(){
		strings.Trim(defaultSearchPath, "/"):           func() { s.handleSearch(w, r) },
		strings.Trim(defaultS3UpstreamsPath, "/"):      func() { s.handleS3Upstreams(w, r) },
		strings.Trim(defaultClusterMembersPath, "/"):   func() { s.handleClusterMembers(w, r) },
		strings.Trim(defaultClusterBootstrapPath, "/"): func() { s.handleClusterBootstrap(w, r) },
		strings.Trim(defaultClusterJoinPath, "/"):      func() { s.handleClusterJoin(w, r) },
		strings.Trim(defaultClusterStatusPath, "/"):    func() { s.handleClusterStatus(w, r) },
		strings.Trim(defaultClusterNodesPath, "/"):     func() { s.handleClusterNodes(w, r) },
		strings.Trim(defaultClusterReconcilePath, "/"): func() { s.handleClusterReconcile(w, r) },
		strings.Trim(defaultClusterRebalancePath, "/"): func() { s.handleClusterRebalance(w, r) },
		strings.Trim(defaultClusterRebalancePlanPath, "/"): func() {
			s.handleClusterRebalancePlan(w, r)
		},
		strings.Trim(defaultClusterStorageNodesPath, "/"): func() {
			s.handleClusterStorageNodes(w, r)
		},
		strings.Trim(defaultClusterStorageNodesSyncPath, "/"): func() {
			s.handleClusterStorageNodesSync(w, r)
		},
		strings.Trim(defaultDiscoveryPath, "/"):     func() { s.handleDiscovery(w, r) },
		strings.Trim(defaultRepairStatusPath, "/"):  func() { s.handleRepairStatus(w, r) },
		strings.Trim(defaultRepairRunPath, "/"):     func() { s.handleRepairRun(w, r) },
		strings.Trim(defaultRepairHistoryPath, "/"): func() { s.handleRepairHistory(w, r) },
		strings.Trim(defaultRepairIssuesPath, "/"):  func() { s.handleRepairIssues(w, r) },
		strings.Trim(defaultDedupeStatusPath, "/"):  func() { s.handleDedupeStatus(w, r) },
		strings.Trim(defaultDedupePlanPath, "/"):    func() { s.handleDedupePlan(w, r) },
		strings.Trim(defaultDedupeRunPath, "/"):     func() { s.handleDedupeRun(w, r) },
		strings.Trim(defaultRecoveryPlanPath, "/"): func() {
			s.handleRecoveryPlan(w, r)
		},
		strings.Trim(defaultRecoveryStatusPath, "/"): func() {
			s.handleRecoveryStatus(w, r)
		},
		strings.Trim(defaultRecoveryRunPath, "/"):  func() { s.handleRecoveryRun(w, r) },
		strings.Trim(defaultIndexStatusPath, "/"):  func() { s.handleIndexStatus(w, r) },
		strings.Trim(defaultIndexRebuildPath, "/"): func() { s.handleIndexRebuild(w, r) },
	}
	if routeHandler, ok := routes[route]; ok {
		routeHandler()
		return true
	}
	if s.handleStorageShardRoute(w, parts) {
		return true
	}
	if isS3UpstreamRoute(parts) {
		s.handleS3Upstream(w, r, parts[2])
		return true
	}
	if isClusterMemberActionRoute(parts) {
		s.handleClusterMemberAction(w, r, parts[2], parts[3])
		return true
	}
	if isClusterMemberRoute(parts) {
		s.handleClusterMember(w, r, parts[2])
		return true
	}
	return false
}

func isHealthRoute(route string) bool {
	return route == "healthz" || route == "health"
}

func isReadinessRoute(route string) bool {
	return route == "readyz" || route == "ready"
}

func isMetricsRoute(route string) bool {
	return route == "metrics"
}

func isClusterMemberRoute(parts []string) bool {
	return len(parts) == 3 && parts[0] == "_cluster" && parts[1] == "members"
}

func isClusterMemberActionRoute(parts []string) bool {
	return len(parts) == 4 && parts[0] == "_cluster" && parts[1] == "members"
}

func isS3UpstreamRoute(parts []string) bool {
	return len(parts) == 3 && parts[0] == "_s3" && parts[1] == "upstreams"
}
