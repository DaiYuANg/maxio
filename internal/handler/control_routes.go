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
	return s.handleNamedControlRoute(w, r, route, parts)
}

func (s *Service) handleNamedControlRoute(w http.ResponseWriter, r *http.Request, route string, parts []string) bool {
	routes := map[string]func(){
		strings.Trim(defaultSearchPath, "/"):            func() { s.handleSearch(w, r) },
		strings.Trim(defaultS3UpstreamsPath, "/"):       func() { s.handleS3Upstreams(w, r) },
		strings.Trim(defaultDedupeStatusPath, "/"):      func() { s.handleDedupeStatus(w, r) },
		strings.Trim(defaultDedupePlanPath, "/"):        func() { s.handleDedupePlan(w, r) },
		strings.Trim(defaultDedupeRunPath, "/"):         func() { s.handleDedupeRun(w, r) },
		strings.Trim(defaultIndexStatusPath, "/"):       func() { s.handleIndexStatus(w, r) },
		strings.Trim(defaultIndexRebuildPath, "/"):      func() { s.handleIndexRebuild(w, r) },
		strings.Trim(defaultProcessingStatusPath, "/"):  func() { s.handleProcessingStatus(w, r) },
		strings.Trim(defaultProcessingRecordsPath, "/"): func() { s.handleProcessingRecord(w, r) },
	}
	if routeHandler, ok := routes[route]; ok {
		routeHandler()
		return true
	}
	if isS3UpstreamRoute(parts) {
		s.handleS3Upstream(w, r, parts[2])
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

func isS3UpstreamRoute(parts []string) bool {
	return len(parts) == 3 && parts[0] == "_s3" && parts[1] == "upstreams"
}
