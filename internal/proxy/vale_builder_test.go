package proxy_test

import (
	"testing"

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

	cfg, err := proxy.BuildValeConfigFromUpstreams(upstreams, proxy.DefaultValeProxyBuildOptions())
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

func TestBuildValeConfigFromUpstreamsRejectsInvalidEndpoint(t *testing.T) {
	upstreams := []model.Upstream{
		{
			ID:       "u-1",
			Name:     "broken",
			Endpoint: "not-a-url",
		},
	}

	_, err := proxy.BuildValeConfigFromUpstreams(upstreams, proxy.DefaultValeProxyBuildOptions())
	if err == nil {
		t.Fatal("expected error for invalid upstream endpoint")
	}
}
