package proxy_test

import (
	"context"
	"testing"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/lyonbrown4d/maxio/internal/proxy"
)

func TestBuildValeConfigFromUpstreamsWithBucketRoutes(t *testing.T) {
	upstreams := []model.Upstream{
		{
			ID:       "u-1",
			Name:     "images",
			Endpoint: "http://127.0.0.1:9000",
			Buckets:  []string{"photos", "/docs", "  docs//api "},
		},
	}

	cfg, err := proxy.BuildValeConfigFromUpstreams(collectionlist.NewList(upstreams...), proxy.DefaultValeProxyBuildOptions())
	if err != nil {
		t.Fatalf("build vale config: %v", err)
	}

	if len(cfg.Entrypoints) != 1 {
		t.Fatalf("expected 1 entrypoint, got %d", len(cfg.Entrypoints))
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(cfg.Services))
	}
	if len(cfg.Routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(cfg.Routes))
	}
}

func TestBuildValeConfigUsesUpstreamIDForDedupeMiddleware(t *testing.T) {
	upstreams := []model.Upstream{
		{
			ID:       "stable-upstream-id",
			Name:     "display-name",
			Endpoint: "http://127.0.0.1:9000",
			Buckets:  []string{"photos"},
		},
	}

	cfg, err := proxy.BuildValeConfigFromUpstreams(collectionlist.NewList(upstreams...), proxy.DefaultValeProxyBuildOptions())
	if err != nil {
		t.Fatalf("build vale config: %v", err)
	}

	hasMiddlewareNamed := func(name string) bool {
		for _, middleware := range cfg.Middlewares {
			if middleware.Name == name {
				return true
			}
		}
		return false
	}
	if !hasMiddlewareNamed("maxio-dedupe:stable-upstream-id") {
		t.Fatal("expected dedupe middleware to use stable upstream id")
	}
	if hasMiddlewareNamed("maxio-dedupe:display-name") {
		t.Fatal("did not expect dedupe middleware to use display name")
	}
}

func TestBuildValeConfigSnapshotAllowsEmptyUpstreams(t *testing.T) {
	cfg, err := proxy.BuildValeConfigSnapshot(collectionlist.NewList[model.Upstream](), proxy.DefaultValeProxyBuildOptions())
	if err != nil {
		t.Fatalf("build vale config snapshot: %v", err)
	}
	if len(cfg.Entrypoints) != 1 {
		t.Fatalf("expected 1 entrypoint, got %d", len(cfg.Entrypoints))
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("expected placeholder service, got %d", len(cfg.Services))
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("expected placeholder route, got %d", len(cfg.Routes))
	}

	provider, err := proxy.NewValeMemoryConfigProvider(cfg)
	if err != nil {
		t.Fatalf("new vale memory config provider: %v", err)
	}
	loaded, err := provider.Load(context.Background())
	if err != nil {
		t.Fatalf("load vale memory config provider: %v", err)
	}
	if len(loaded.Entrypoints) != 1 {
		t.Fatalf("expected loaded entrypoint, got %d", len(loaded.Entrypoints))
	}
}

func TestBuildValeConfigFromUpstreamsRejectsInvalidEndpoint(t *testing.T) {
	upstreams := []model.Upstream{
		{
			ID:       "u-1",
			Name:     "broken",
			Endpoint: "not-a-url",
		},
	}

	_, err := proxy.BuildValeConfigFromUpstreams(collectionlist.NewList(upstreams...), proxy.DefaultValeProxyBuildOptions())
	if err == nil {
		t.Fatal("expected error for invalid upstream endpoint")
	}
}
