package handler_test

import (
	"context"
	"log/slog"
	"net/http"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
)

func TestObjectMutationAuditIncludesStructuredFields(t *testing.T) {
	capture := &auditCaptureHandler{}
	router := newObjectRouter(t, config.Config{APIToken: "api-secret"}, slog.New(capture))
	headers := map[string]string{"X-Maxio-API": "api-secret"}

	recorder := serveRouterRequest(router, http.MethodPut, "/audit-bucket", headers, nil)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create bucket status = %d body = %s, want %d", recorder.Code, recorder.Body.String(), http.StatusCreated)
	}

	headers = map[string]string{
		"X-Maxio-API":  "api-secret",
		"X-Request-ID": "audit-request-1",
		"User-Agent":   "maxio-auth-test",
	}
	recorder = serveRouterRequest(router, http.MethodPut, "/audit-bucket/doc.txt", headers, []byte("payload"))
	if recorder.Code != http.StatusOK {
		t.Fatalf("put object status = %d body = %s, want %d", recorder.Code, recorder.Body.String(), http.StatusOK)
	}

	record, ok := capture.find("object.put")
	if !ok {
		t.Fatalf("object.put audit event not found in records: %+v", capture.records)
	}
	assertAuditField(t, record, "audit_category", "object_mutation")
	assertAuditField(t, record, "audit_outcome", "success")
	assertAuditField(t, record, "request_id", "audit-request-1")
	assertAuditField(t, record, "auth_principal_type", "api")
	assertAuditField(t, record, "auth_credential", "x-maxio-api")
	assertAuditField(t, record, "method", http.MethodPut)
	assertAuditField(t, record, "path", "/audit-bucket/doc.txt")
	assertAuditField(t, record, "user_agent", "maxio-auth-test")
	assertAuditField(t, record, "bucket", "audit-bucket")
	assertAuditField(t, record, "key", "doc.txt")
	assertAuditField(t, record, "size", int64(len("payload")))
	if record["etag"] == "" {
		t.Fatal("expected object.put audit event to include etag")
	}
}

type auditCaptureHandler struct {
	records []map[string]any
}

func (h *auditCaptureHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *auditCaptureHandler) Handle(_ context.Context, record slog.Record) error {
	fields := map[string]any{"message": record.Message}
	record.Attrs(func(attr slog.Attr) bool {
		fields[attr.Key] = attr.Value.Any()
		return true
	})
	h.records = append(h.records, fields)
	return nil
}

func (h *auditCaptureHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *auditCaptureHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *auditCaptureHandler) find(action string) (map[string]any, bool) {
	for index := range h.records {
		if h.records[index]["audit_action"] == action {
			return h.records[index], true
		}
	}
	return nil, false
}

func assertAuditField(t *testing.T, record map[string]any, key string, want any) {
	t.Helper()

	if got := record[key]; got != want {
		t.Fatalf("audit field %s = %#v, want %#v", key, got, want)
	}
}
