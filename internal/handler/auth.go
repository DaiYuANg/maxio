package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const maxioControlHeader = "X-Maxio-Control"
const maxioClusterHeader = "X-Maxio-Cluster"

func (s *Service) requiresClusterAuth(parts []string) bool {
	return isStorageShardRoute(parts) && strings.TrimSpace(s.cfg.ClusterToken) != ""
}

func (s *Service) requiresAdminAuth(route string, parts []string) bool {
	if strings.TrimSpace(s.cfg.AdminToken) == "" {
		return false
	}
	if s.requiresClusterAuth(parts) {
		return false
	}
	if route == strings.Trim(defaultSearchPath, "/") || route == "metrics" {
		return true
	}
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case "_cluster", "_repair", "_dedupe", "_index", "_internal":
		return true
	default:
		return false
	}
}

func (s *Service) requiresAPIAuth(route string, parts []string) bool {
	if strings.TrimSpace(s.cfg.APIToken) == "" || s.requiresAdminAuth(route, parts) {
		return false
	}
	if isHealthRoute(route) || isReadinessRoute(route) {
		return false
	}
	return true
}

func (s *Service) authorizeAdmin(r *http.Request) bool {
	token := strings.TrimSpace(s.cfg.AdminToken)
	if token == "" {
		return true
	}
	provided := adminTokenFromRequest(r)
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func (s *Service) authorizeCluster(r *http.Request) bool {
	token := strings.TrimSpace(s.cfg.ClusterToken)
	if token == "" {
		return true
	}
	provided := clusterTokenFromRequest(r)
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func (s *Service) authorizeAPI(r *http.Request) bool {
	token := strings.TrimSpace(s.cfg.APIToken)
	if token == "" {
		return true
	}
	provided := apiCredentialFromRequest(r)
	if provided == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1 {
		return true
	}
	adminToken := strings.TrimSpace(s.cfg.AdminToken)
	return adminToken != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(adminToken)) == 1
}

func adminTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(r.Header.Get(maxioControlHeader)); value != "" {
		return value
	}
	return bearerTokenFromRequest(r)
}

func clusterTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(r.Header.Get(maxioClusterHeader)); value != "" {
		return value
	}
	return bearerTokenFromRequest(r)
}

func bearerTokenFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[len("bearer "):])
	}
	return ""
}

func apiCredentialFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	if value := strings.TrimSpace(r.Header.Get("X-Maxio-API")); value != "" {
		return value
	}
	return adminTokenFromRequest(r)
}

func (s *Service) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="maxio-admin"`)
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin authorization required"})
}

func (s *Service) writeClusterUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="maxio-cluster"`)
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "cluster authorization required"})
}

func (s *Service) storageNodeToken() string {
	if token := strings.TrimSpace(s.cfg.ClusterToken); token != "" {
		return token
	}
	return strings.TrimSpace(s.cfg.AdminToken)
}
