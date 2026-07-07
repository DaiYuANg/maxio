package handler

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const maxioControlHeader = "X-Maxio-Control"

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

	authSourceNone          = "none"
	authSourceBearer        = "authorization-bearer"
	authSourceControlHeader = "x-maxio-control"
)

func (s *Service) requiresAdminAuth(route string, parts []string) bool {
	if strings.TrimSpace(s.cfg.AdminToken) == "" {
		return false
	}
	if route == strings.Trim(defaultSearchPath, "/") || route == "metrics" {
		return true
	}
	if len(parts) == 0 {
		return false
	}
	switch parts[0] {
	case "_dedupe", "_index", "_processing", "_s3":
		return true
	default:
		return false
	}
}

func (s *Service) authorizeControlHTTPRequest(
	w http.ResponseWriter,
	r *http.Request,
	route string,
	parts []string,
) bool {
	if s.requiresAdminAuth(route, parts) && !s.authorizeAdmin(r) {
		s.writeUnauthorized(w)
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

func (s *Service) requestAuthPrincipal(r *http.Request) authPrincipal {
	if s == nil || r == nil {
		return authPrincipal{kind: authPrincipalAnonymous, source: authSourceNone}
	}
	if credential := adminCredentialFromRequest(r); tokenMatches(credential.value, s.cfg.AdminToken) {
		return authPrincipal{kind: authPrincipalAdmin, source: credential.source}
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

func (s *Service) writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="maxio-admin"`)
	s.writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "admin authorization required"})
}
