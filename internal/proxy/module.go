package proxy

import (
	"context"
	"log/slog"
	"strings"

	"github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dix"
	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vale"
	valeconfig "github.com/arcgolabs/vale/config"
	"github.com/lyonbrown4d/maxio/internal/cache"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/oops"
)

const (
	defaultEnabledProxyEntrypoint = ":8080"
)

type ValeRuntime struct {
	gateway        *vale.Gateway
	configProvider valeConfigUpdater
	store          metadata.MetadataStore
	options        ValeProxyBuildOptions
	enabled        bool
	logger         *slog.Logger
}

type valeConfigUpdater interface {
	Update(cfgData *valeconfig.Config) error
}

// Module wires the Vale reverse-proxy runtime.
//
// When S3 proxy is disabled, the module registers an empty runtime so application
// startup remains unchanged.
func Module() dix.Module {
	return dix.NewModule(
		"proxy",
		dix.WithModuleProviders(
			dix.ProviderErr5(newValeGateway),
		),
		dix.Hooks(
			dix.OnStart(startValeGateway),
			dix.OnStop(stopValeGateway),
		),
	)
}

func newValeGateway(
	cfg config.Config,
	logger *slog.Logger,
	store metadata.MetadataStore,
	bus eventx.BusRuntime,
	metadataCache cache.MetadataCache,
) (*ValeRuntime, error) {
	if !cfg.EnableS3Proxy {
		return &ValeRuntime{}, nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	if err := seedConfiguredUpstreams(context.Background(), store, cfg.S3ProxyUpstreams); err != nil {
		return nil, oops.Wrapf(err, "seed configured upstreams")
	}
	upstreams, err := loadEnabledUpstreams(context.Background(), store)
	if err != nil {
		return nil, oops.Wrapf(err, "load upstreams")
	}
	if upstreams == nil || upstreams.Len() == 0 {
		logger.Warn("s3 proxy enabled without configured upstreams")
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
	cfgData, err := BuildValeConfigSnapshot(upstreams, options)
	if err != nil {
		return nil, oops.Wrapf(err, "build vale config")
	}

	configProvider, err := NewValeMemoryConfigProvider(cfgData)
	if err != nil {
		return nil, oops.Wrapf(err, "new vale config provider")
	}
	gateway, err := BuildValeGatewayFromProvider(configProvider, logger, NewDedupeMiddlewareRegistry(bus, store, metadataCache, logger))
	if err != nil {
		return nil, oops.Wrapf(err, "new vale gateway")
	}
	return &ValeRuntime{
		gateway:        gateway,
		configProvider: configProvider,
		store:          store,
		options:        options,
		enabled:        true,
		logger:         logger,
	}, nil
}

func (r *ValeRuntime) Reload(ctx context.Context) error {
	if r == nil || !r.enabled || r.configProvider == nil {
		return nil
	}
	upstreams, err := loadEnabledUpstreams(ctx, r.store)
	if err != nil {
		return oops.Wrapf(err, "load upstreams")
	}
	if (upstreams == nil || upstreams.Len() == 0) && r.logger != nil {
		r.logger.WarnContext(ctx, "reloading s3 proxy without enabled upstreams")
	}
	cfgData, err := BuildValeConfigSnapshot(upstreams, r.options)
	if err != nil {
		return oops.Wrapf(err, "build vale config")
	}
	if err := r.configProvider.Update(cfgData); err != nil {
		return oops.Wrapf(err, "update vale config provider")
	}
	return nil
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
		return oops.Wrapf(err, "get upstream %q", id)
	} else if ok {
		return nil
	}
	if _, err := store.UpsertUpstream(ctx, upstream); err != nil {
		return oops.Wrapf(err, "upsert upstream %q", id)
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

func loadEnabledUpstreams(ctx context.Context, store metadata.MetadataStore) (*list.List[model.Upstream], error) {
	if store == nil {
		return list.NewList[model.Upstream](), nil
	}
	upstreams, err := store.ListUpstreams(ctx)
	if err != nil {
		return nil, oops.Wrapf(err, "list upstreams")
	}
	if upstreams == nil {
		return list.NewList[model.Upstream](), nil
	}
	return list.Where(upstreams, func(_ int, upstream model.Upstream) bool {
		return upstream.Enabled
	}), nil
}

func startValeGateway(ctx context.Context, runtime *ValeRuntime) error {
	if runtime == nil || runtime.gateway == nil {
		return nil
	}
	if err := runtime.gateway.Start(ctx); err != nil {
		return oops.Wrapf(err, "start vale gateway")
	}
	return nil
}

func stopValeGateway(ctx context.Context, runtime *ValeRuntime) error {
	if runtime == nil || runtime.gateway == nil {
		return nil
	}
	if err := runtime.gateway.Stop(ctx); err != nil {
		return oops.Wrapf(err, "stop vale gateway")
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
