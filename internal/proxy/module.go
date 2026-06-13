package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/vale"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const (
	defaultEnabledProxyEntrypoint = ":8080"
)

type ValeRuntime struct {
	gateway *vale.Gateway
}

// Module wires the Vale reverse-proxy runtime.
//
// When S3 proxy is disabled, the module registers an empty runtime so application
// startup remains unchanged.
func Module() dix.Module {
	return dix.NewModule(
		"proxy",
		dix.WithModuleProviders(
			dix.ProviderErr3(newValeGateway),
		),
		dix.Hooks(
			dix.OnStart(startValeGateway),
			dix.OnStop(stopValeGateway),
		),
	)
}

func newValeGateway(cfg config.Config, logger *slog.Logger, store metadata.MetadataStore) (*ValeRuntime, error) {
	if !cfg.EnableS3Proxy {
		return &ValeRuntime{}, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := seedConfiguredUpstreams(context.Background(), store, cfg.S3ProxyUpstreams); err != nil {
		return nil, fmt.Errorf("seed configured upstreams: %w", err)
	}
	upstreams, err := loadEnabledUpstreams(context.Background(), store)
	if err != nil {
		return nil, fmt.Errorf("load upstreams: %w", err)
	}
	if len(upstreams) == 0 {
		logger.Warn("s3 proxy enabled without configured upstreams")
		return &ValeRuntime{}, nil
	}

	options := ValeProxyBuildOptions{
		Entrypoint:        cfg.S3ProxyEntrypoint,
		EntrypointName:    "web",
		AdminAddress:      cfg.S3ProxyAdminAddress,
		HealthInterval:    cfg.S3ProxyHealthInterval,
		HealthTimeout:     cfg.S3ProxyHealthTimeout,
		EnableHealthCheck: true,
	}
	options = normalizeValeOptionsWithDefaults(options)
	cfgData, err := BuildValeConfigFromUpstreams(upstreams, options)
	if err != nil {
		return nil, fmt.Errorf("build vale config: %w", err)
	}

	gateway, err := BuildValeGatewayFromConfig(cfgData, logger)
	if err != nil {
		return nil, fmt.Errorf("new vale gateway: %w", err)
	}
	return &ValeRuntime{gateway: gateway}, nil
}

func seedConfiguredUpstreams(ctx context.Context, store metadata.MetadataStore, upstreams []model.Upstream) error {
	if store == nil {
		return nil
	}
	for i := range upstreams {
		if err := seedConfiguredUpstream(ctx, store, upstreams[i]); err != nil {
			return err
		}
	}
	return nil
}

func seedConfiguredUpstream(ctx context.Context, store metadata.MetadataStore, upstream model.Upstream) error {
	upstream = seedUpstreamDefaults(upstream)
	id := upstreamSeedID(upstream)
	if id == "" {
		return metadata.ErrBadRequest
	}
	if _, ok, err := store.GetUpstream(ctx, id); err != nil {
		return fmt.Errorf("get upstream %q: %w", id, err)
	} else if ok {
		return nil
	}
	if _, err := store.UpsertUpstream(ctx, upstream); err != nil {
		return fmt.Errorf("upsert upstream %q: %w", id, err)
	}
	return nil
}

func upstreamSeedID(upstream model.Upstream) string {
	id := strings.TrimSpace(upstream.ID)
	if id != "" {
		return id
	}
	return strings.TrimSpace(upstream.Name)
}

func seedUpstreamDefaults(upstream model.Upstream) model.Upstream {
	upstream.ID = strings.TrimSpace(upstream.ID)
	upstream.Name = strings.TrimSpace(upstream.Name)
	if upstream.ID == "" {
		upstream.ID = upstream.Name
	}
	if upstream.Name == "" {
		upstream.Name = upstream.ID
	}
	upstream.Enabled = true
	if upstream.Weight == 0 {
		upstream.Weight = 1
	}
	return upstream
}

func loadEnabledUpstreams(ctx context.Context, store metadata.MetadataStore) ([]model.Upstream, error) {
	if store == nil {
		return nil, nil
	}
	upstreams, err := store.ListUpstreams(ctx)
	if err != nil {
		return nil, fmt.Errorf("list upstreams: %w", err)
	}
	enabled := make([]model.Upstream, 0, len(upstreams))
	for i := range upstreams {
		upstream := upstreams[i]
		if upstream.Enabled {
			enabled = append(enabled, upstream)
		}
	}
	return enabled, nil
}

func startValeGateway(ctx context.Context, runtime *ValeRuntime) error {
	if runtime == nil || runtime.gateway == nil {
		return nil
	}
	if err := runtime.gateway.Start(ctx); err != nil {
		return fmt.Errorf("start vale gateway: %w", err)
	}
	return nil
}

func stopValeGateway(ctx context.Context, runtime *ValeRuntime) error {
	if runtime == nil || runtime.gateway == nil {
		return nil
	}
	if err := runtime.gateway.Stop(ctx); err != nil {
		return fmt.Errorf("stop vale gateway: %w", err)
	}
	return nil
}

func normalizeValeOptionsWithDefaults(options ValeProxyBuildOptions) ValeProxyBuildOptions {
	normalized := normalizeValeOptions(options)
	if normalized.Entrypoint == "" {
		normalized.Entrypoint = defaultEnabledProxyEntrypoint
	}
	return normalized
}
