package handler_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/engine"
	"github.com/lyonbrown4d/maxio/internal/handler"
	"github.com/spf13/afero"
)

func TestAdminAuthDisabledByDefault(t *testing.T) {
	recorder := serveHandlerGet(t, config.Config{}, "/metrics", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAdminAuthProtectsControlRoutes(t *testing.T) {
	cfg := config.Config{AdminToken: "secret"}
	recorder := serveHandlerGet(t, cfg, "/metrics", nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestAdminAuthAcceptsBearerCredential(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer secret"}
	recorder := serveHandlerGet(t, config.Config{AdminToken: "secret"}, "/metrics", headers)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAdminAuthAcceptsControlHeader(t *testing.T) {
	headers := map[string]string{"X-Maxio-Control": "secret"}
	recorder := serveHandlerGet(t, config.Config{AdminToken: "secret"}, "/metrics", headers)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestAdminAuthDoesNotProtectHealth(t *testing.T) {
	recorder := serveHandlerGet(t, config.Config{AdminToken: "secret"}, "/healthz", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestRequestIDGenerated(t *testing.T) {
	recorder := serveHandlerGet(t, config.Config{}, "/healthz", nil)
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected generated request id header")
	}
}

func TestRequestIDPreserved(t *testing.T) {
	headers := map[string]string{"X-Request-ID": "client-request-1"}
	recorder := serveHandlerGet(t, config.Config{}, "/healthz", headers)
	if recorder.Header().Get("X-Request-ID") != "client-request-1" {
		t.Fatalf("request id = %q, want client-request-1", recorder.Header().Get("X-Request-ID"))
	}
}

func TestAPIAuthProtectsObjectRoutes(t *testing.T) {
	recorder := serveHandlerGet(t, config.Config{APIToken: "api-secret"}, "/bucket", nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestAPIAuthDoesNotProtectReadiness(t *testing.T) {
	recorder := serveHandlerGet(t, config.Config{APIToken: "api-secret"}, "/readyz", nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestClusterAuthRejectsStorageShardRouteWithoutToken(t *testing.T) {
	recorder := serveStorageShardPut(t, config.Config{ClusterToken: "cluster-secret"}, nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestClusterAuthAcceptsStorageShardHeader(t *testing.T) {
	headers := map[string]string{"X-Maxio-Cluster": "cluster-secret"}
	recorder := serveStorageShardPut(t, config.Config{ClusterToken: "cluster-secret"}, headers)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestClusterAuthAcceptsStorageShardBearerToken(t *testing.T) {
	headers := map[string]string{"Authorization": "Bearer cluster-secret"}
	recorder := serveStorageShardPut(t, config.Config{ClusterToken: "cluster-secret"}, headers)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestClusterAuthRejectsStorageShardRouteWithWrongToken(t *testing.T) {
	headers := map[string]string{"X-Maxio-Cluster": "wrong-secret"}
	recorder := serveStorageShardPut(t, config.Config{ClusterToken: "cluster-secret"}, headers)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestClusterAuthDoesNotAcceptAdminTokenForStorageShardRoute(t *testing.T) {
	headers := map[string]string{"X-Maxio-Control": "admin-secret"}
	cfg := config.Config{AdminToken: "admin-secret", ClusterToken: "cluster-secret"}
	recorder := serveStorageShardPut(t, cfg, headers)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func serveHandlerGet(
	t *testing.T,
	cfg config.Config,
	target string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	service := handler.NewService(handler.Dependencies{}, slog.New(slog.DiscardHandler), cfg)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func serveStorageShardPut(
	t *testing.T,
	cfg config.Config,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, afero.NewMemMapFs())
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}
	deps := handler.NewDependencies(nil, eng, nil, nil, nil, nil)
	service := handler.NewService(deps, slog.New(slog.DiscardHandler), cfg)
	router := http.NewServeMux()
	service.RegisterHTTP(router)

	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPut,
		"/_internal/storage/shards/ab/hash-1/0",
		bytes.NewReader([]byte("payload")),
	)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}
