package handler

import (
	"net/http"
	"strings"
)

func (s *Service) auditHTTP(r *http.Request, action string, attrs ...any) {
	if s == nil || s.logger == nil || r == nil {
		return
	}
	principal := s.requestAuthPrincipal(r)
	fields := make([]any, 0, 18+len(attrs))
	fields = append(fields,
		"audit_action", action,
		"audit_category", auditCategory(action),
		"audit_outcome", "success",
		"request_id", requestIDFromContext(r.Context()),
		"auth_principal_type", principal.kind,
		"auth_credential", principal.source,
		"method", r.Method,
		"path", r.URL.Path,
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent(),
	)
	fields = append(fields, attrs...)
	s.logger.InfoContext(r.Context(), "audit event", fields...)
}

func auditCategory(action string) string {
	switch {
	case strings.HasPrefix(action, "bucket."), strings.HasPrefix(action, "object."):
		return "object_mutation"
	case strings.HasPrefix(action, "cluster."),
		strings.HasPrefix(action, "dedupe."),
		strings.HasPrefix(action, "index."):
		return "admin_mutation"
	default:
		return "operation"
	}
}
