package handler_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/handler"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/object"
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

func TestAdminAuthProtectsRecoveryRoutes(t *testing.T) {
	recorder := serveHandlerGet(t, config.Config{AdminToken: "secret"}, "/_recovery/status", nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
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

func TestNativeObjectAPIDisabledRejectsBucketRoute(t *testing.T) {
	cfg := config.Config{EnableNativeObjectAPI: false}
	recorder := serveObjectRequest(t, cfg, http.MethodGet, "/photos", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestNativeObjectAPIDisabledRejectsObjectRoute(t *testing.T) {
	cfg := config.Config{EnableNativeObjectAPI: false}
	recorder := serveObjectRequest(t, cfg, http.MethodGet, "/photos/cat.jpg", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestAPIAuthDoesNotProtectReadiness(t *testing.T) {
	recorder := serveHandlerGet(t, config.Config{APIToken: "api-secret"}, "/readyz", nil)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestAPITokenDoesNotAuthorizeAdminRoutes(t *testing.T) {
	headers := map[string]string{"X-Maxio-API": "api-secret"}
	cfg := config.Config{AdminToken: "admin-secret", APIToken: "api-secret"}
	recorder := serveHandlerGet(t, cfg, "/metrics", headers)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestObjectAuthAcceptsAPIHeaderForBucketMutation(t *testing.T) {
	headers := map[string]string{"X-Maxio-API": "api-secret"}
	recorder := serveObjectRequest(
		t,
		config.Config{APIToken: "api-secret"},
		http.MethodPut,
		"/photos",
		headers,
	)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s, want %d", recorder.Code, recorder.Body.String(), http.StatusCreated)
	}
}

func TestObjectAuthAcceptsAdminTokenForBucketMutation(t *testing.T) {
	headers := map[string]string{"X-Maxio-Control": "admin-secret"}
	cfg := config.Config{AdminToken: "admin-secret", APIToken: "api-secret"}
	recorder := serveObjectRequest(t, cfg, http.MethodPut, "/admin-photos", headers)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s, want %d", recorder.Code, recorder.Body.String(), http.StatusCreated)
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

func serveObjectRequest(
	t *testing.T,
	cfg config.Config,
	method string,
	target string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()

	return serveRouterRequest(newObjectRouter(t, cfg, slog.New(slog.DiscardHandler)), method, target, headers, nil)
}

func newObjectRouter(t *testing.T, cfg config.Config, logger *slog.Logger) http.Handler {
	t.Helper()

	storage, err := store.NewStore(t.TempDir(), metadata.NewInMemoryMetadata(), nil)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	objects := object.NewService(storage, nil, nil, slog.New(slog.DiscardHandler), object.Config{})
	deps := handler.NewDependencies(objects, nil, nil, nil)
	service := handler.NewService(deps, logger, cfg)
	router := http.NewServeMux()
	service.RegisterHTTP(router)
	return router
}

func serveRouterRequest(
	router http.Handler,
	method string,
	target string,
	headers map[string]string,
	body []byte,
) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(context.Background(), method, target, bytes.NewReader(body))
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
	deps := handler.NewDependencies(nil, eng, nil, nil)
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
