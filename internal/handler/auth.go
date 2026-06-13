package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const maxioControlHeader = "X-Maxio-Control"
const maxioClusterHeader = "X-Maxio-Cluster"

type authCredential struct {
	value  string
	source string
}

type authPrincipal struct {
	kind   string
	source string
}

const (
	authPrincipalAnonymous = "anonymous"
	authPrincipalAdmin     = "admin"
	authPrincipalAPI       = "api"
	authPrincipalCluster   = "cluster"

	authSourceNone          = "none"
	authSourceBearer        = "authorization-bearer"
	authSourceControlHeader = "x-maxio-control"
	authSourceClusterHeader = "x-maxio-cluster"
	authSourceAPIHeader     = "x-maxio-api"
)

type objectPermission string

const (
	objectPermissionRead  objectPermission = "object:read"
	objectPermissionWrite objectPermission = "object:write"
)

type objectAuthorization struct {
	permission objectPermission
	resource   string
	bucket     string
	key        string
}

func (s *Service) requiresClusterAuth(parts []string) bool {
	_ = parts
	return false
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
	case "_cluster", "_dedupe", "_index", "_s3":
		return true
	default:
		return false
	}
}

func (s *Service) requiresObjectAuth(route string, parts []string) bool {
	if strings.TrimSpace(s.cfg.APIToken) == "" || s.requiresAdminAuth(route, parts) {
		return false
	}
	if isHealthRoute(route) || isReadinessRoute(route) {
		return false
	}
	return true
}

func (s *Service) authorizeControlHTTPRequest(
	w http.ResponseWriter,
	r *http.Request,
	route string,
	parts []string,
) bool {
	if s.requiresClusterAuth(parts) && !s.authorizeCluster(r) {
		s.writeClusterUnauthorized(w)
		return false
	}
	if s.requiresAdminAuth(route, parts) && !s.authorizeAdmin(r) {
		s.writeUnauthorized(w)
		return false
	}
	return true
}

func (s *Service) authorizeNativeObjectHTTPRequest(
	w http.ResponseWriter,
	r *http.Request,
	route string,
	parts []string,
) bool {
	objectAuthz := objectAuthorizationForRoute(route, parts, r.Method)
	if s.requiresObjectAuth(route, parts) && !s.authorizeObject(r, objectAuthz) {
		s.writeAPIUnauthorized(w)
		return false
	}
	return true
}

func (s *Service) authorizeAdmin(r *http.Request) bool {
	if strings.TrimSpace(s.cfg.AdminToken) == "" {
		return true
	}
	return s.requestAuthPrincipal(r).kind == authPrincipalAdmin
}

func (s *Service) authorizeCluster(r *http.Request) bool {
	if strings.TrimSpace(s.cfg.ClusterToken) == "" {
		return true
	}
	return s.requestAuthPrincipal(r).kind == authPrincipalCluster
}

func (s *Service) authorizeObject(r *http.Request, _ objectAuthorization) bool {
	if strings.TrimSpace(s.cfg.APIToken) == "" {
		return true
	}
	principal := s.requestAuthPrincipal(r)
	switch principal.kind {
	case authPrincipalAdmin, authPrincipalAPI:
		return true
	default:
		return false
	}
}

func objectAuthorizationForRoute(route string, parts []string, method string) objectAuthorization {
	authz := objectAuthorization{
		permission: objectPermissionForMethod(method),
		resource:   "bucket_collection",
	}
	if route == "" {
		return authz
	}
	if len(parts) == 1 {
		authz.resource = "bucket"
		authz.bucket = parts[0]
		return authz
	}
	authz.resource = "object"
	authz.bucket = parts[0]
	authz.key = strings.Join(parts[1:], "/")
	return authz
}

func objectPermissionForMethod(method string) objectPermission {
	switch method {
	case http.MethodGet, http.MethodHead:
		return objectPermissionRead
	default:
		return objectPermissionWrite
	}
}

func (s *Service) requestAuthPrincipal(r *http.Request) authPrincipal {
	if s == nil || r == nil {
		return authPrincipal{kind: authPrincipalAnonymous, source: authSourceNone}
	}
	if credential := clusterCredentialFromRequest(r); tokenMatches(credential.value, s.cfg.ClusterToken) {
		return authPrincipal{kind: authPrincipalCluster, source: credential.source}
	}
	if credential := adminCredentialFromRequest(r); tokenMatches(credential.value, s.cfg.AdminToken) {
		return authPrincipal{kind: authPrincipalAdmin, source: credential.source}
	}
	if credential := apiCredentialFromRequestDetails(r); tokenMatches(credential.value, s.cfg.APIToken) {
		return authPrincipal{kind: authPrincipalAPI, source: credential.source}
	}
	return authPrincipal{kind: authPrincipalAnonymous, source: authSourceNone}
}

func tokenMatches(provided, expected string) bool {
	provided = strings.TrimSpace(provided)
	expected = strings.TrimSpace(expected)
	if provided == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func adminCredentialFromRequest(r *http.Request) authCredential {
	if r == nil {
		return authCredential{source: authSourceNone}
	}
	if value := strings.TrimSpace(r.Header.Get(maxioControlHeader)); value != "" {
		return authCredential{value: value, source: authSourceControlHeader}
	}
	return bearerCredentialFromRequest(r)
}

func clusterCredentialFromRequest(r *http.Request) authCredential {
	if r == nil {
		return authCredential{source: authSourceNone}
	}
	if value := strings.TrimSpace(r.Header.Get(maxioClusterHeader)); value != "" {
		return authCredential{value: value, source: authSourceClusterHeader}
	}
	return bearerCredentialFromRequest(r)
}

func bearerCredentialFromRequest(r *http.Request) authCredential {
	if r == nil {
		return authCredential{source: authSourceNone}
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return authCredential{value: strings.TrimSpace(auth[len("bearer "):]), source: authSourceBearer}
	}
	return authCredential{source: authSourceNone}
}

func apiCredentialFromRequestDetails(r *http.Request) authCredential {
	if r == nil {
		return authCredential{source: authSourceNone}
	}
	if value := strings.TrimSpace(r.Header.Get("X-Maxio-API")); value != "" {
		return authCredential{value: value, source: authSourceAPIHeader}
	}
	return bearerCredentialFromRequest(r)
}

func (s *Service) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="maxio-admin"`)
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin authorization required"})
}

func (s *Service) writeClusterUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="maxio-cluster"`)
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "cluster authorization required"})
}

func (s *Service) writeAPIUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="maxio-api"`)
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "api authorization required"})
}
