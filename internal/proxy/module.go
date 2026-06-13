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

// Module wires the Vale reverse-proxy runtime.
//
// When S3 proxy is disabled or no upstreams are configured, the module registers
// a nil gateway so application startup remains unchanged.
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

func newValeGateway(cfg config.Config, logger *slog.Logger) (*vale.Gateway, error) {
	if !cfg.EnableS3Proxy {
		return nil, nil
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
	return gateway, nil
}

func startValeGateway(ctx context.Context, gateway *vale.Gateway) error {
	if gateway == nil {
		return nil
	}
	return gateway.Start(ctx)
}

func stopValeGateway(_ context.Context, gateway *vale.Gateway) error {
	if gateway == nil {
		return nil
	}
	return gateway.Stop(context.Background())
}

func normalizeValeOptionsWithDefaults(options ValeProxyBuildOptions) ValeProxyBuildOptions {
	normalized := normalizeValeOptions(options)
	if normalized.Entrypoint == "" {
		normalized.Entrypoint = defaultEnabledProxyEntrypoint
	}
	return normalized
}
