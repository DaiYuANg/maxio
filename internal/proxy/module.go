package proxy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/vale"
	"github.com/lyonbrown4d/maxio/internal/config"
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
			dix.ProviderErr2(newValeGateway),
		),
		dix.Hooks(
			dix.OnStart(startValeGateway),
			dix.OnStop(stopValeGateway),
		),
	)
}

func newValeGateway(cfg config.Config, logger *slog.Logger) (*ValeRuntime, error) {
	if !cfg.EnableS3Proxy {
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
	cfgData, err := BuildValeConfigFromUpstreams(cfg.S3ProxyUpstreams, options)
	if err != nil {
		return nil, fmt.Errorf("build vale config: %w", err)
	}

	gateway, err := BuildValeGatewayFromConfig(cfgData, logger)
	if err != nil {
		return nil, fmt.Errorf("new vale gateway: %w", err)
	}
	return &ValeRuntime{gateway: gateway}, nil
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
