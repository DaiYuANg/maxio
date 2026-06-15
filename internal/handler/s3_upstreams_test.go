package handler_test

import (
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/handler"
	"github.com/lyonbrown4d/maxio/internal/metadata"
)

func TestS3UpstreamRoutesCreateListGetAndDelete(t *testing.T) {
	router := newS3UpstreamRouter(t, config.Config{})

	body := []byte(`{"id":"u-1","name":"primary","endpoint":"http://127.0.0.1:9000","buckets":["photos"]}`)
	create := serveRouterRequest(router, http.MethodPost, "/_s3/upstreams", body)
	if create.Code != http.StatusOK {
		t.Fatalf("create status = %d body = %s", create.Code, create.Body.String())
	}

	list := serveRouterRequest(router, http.MethodGet, "/_s3/upstreams", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list status = %d body = %s", list.Code, list.Body.String())
	}
	if !strings.Contains(list.Body.String(), `"id":"u-1"`) {
		t.Fatalf("list body missing upstream: %s", list.Body.String())
	}

	get := serveRouterRequest(router, http.MethodGet, "/_s3/upstreams/u-1", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", get.Code, get.Body.String())
	}

	del := serveRouterRequest(router, http.MethodDelete, "/_s3/upstreams/u-1", nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d body = %s", del.Code, del.Body.String())
	}
}

func TestS3UpstreamRoutesRequireAdminAuth(t *testing.T) {
	router := newS3UpstreamRouter(t, config.Config{AdminToken: "secret"})

	recorder := serveRouterRequest(router, http.MethodGet, "/_s3/upstreams", nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func newS3UpstreamRouter(t *testing.T, cfg config.Config) http.Handler {
	t.Helper()

	deps := handler.NewDependencies(metadata.NewInMemoryMetadata())
	service := handler.NewService(deps, slog.New(slog.DiscardHandler), cfg)
	router := http.NewServeMux()
	service.RegisterHTTP(router)
	return router
}
