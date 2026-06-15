package proxy

import (
	"log/slog"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/arcgolabs/vale"
	valeconfig "github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	"github.com/arcgolabs/vale/provider/memoryconfig"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/mo"
	"github.com/samber/oops"
)

const defaultValeEntrypoint = "web"
const defaultValeEntrypointAddress = ":8080"
const defaultValeAdminAddress = ":19090"
const defaultValeHealthInterval = "5s"
const defaultValeHealthTimeout = "2s"
const disabledValeServiceName = "maxio-disabled"
const disabledValeRouteName = "maxio-disabled"
const disabledValeRoutePath = "/__maxio_s3_proxy_disabled__/"
const disabledValeEndpoint = "http://127.0.0.1:1"

type ValeProxyBuildOptions struct {
	EntrypointName    string
	Entrypoint        string
	AdminAddress      string
	EnableAccessLog   bool
	EnableMetrics     bool
	EnableHealthCheck bool
	HealthInterval    string
	HealthTimeout     string
}

func DefaultValeProxyBuildOptions() ValeProxyBuildOptions {
	return ValeProxyBuildOptions{
		EntrypointName:    defaultValeEntrypoint,
		Entrypoint:        defaultValeEntrypointAddress,
		AdminAddress:      defaultValeAdminAddress,
		EnableAccessLog:   true,
		EnableMetrics:     true,
		EnableHealthCheck: true,
		HealthInterval:    defaultValeHealthInterval,
		HealthTimeout:     defaultValeHealthTimeout,
	}
}

func BuildValeConfigFromUpstreams(upstreams []model.Upstream, options ValeProxyBuildOptions) (*valeconfig.Config, error) {
	if len(upstreams) == 0 {
		err := oops.New("vale upstream list is empty")
		return nil, oops.Wrapf(err, "validate vale upstreams")
	}
	return BuildValeConfigSnapshot(upstreams, options)
}

func BuildValeConfigSnapshot(upstreams []model.Upstream, options ValeProxyBuildOptions) (*valeconfig.Config, error) {
	options = normalizeValeOptions(options)

	b := provider.NewConfigBuilder()
	b.Entrypoint(options.EntrypointName, options.Entrypoint).
		Admin(options.AdminAddress).
		Observability(options.EnableAccessLog, options.EnableMetrics)
	if options.EnableHealthCheck {
		b.Health(options.HealthInterval, options.HealthTimeout)
	}

	bucketsByUpstream := sortUpstreamsForDeterministicBuild(upstreams)
	if len(bucketsByUpstream) == 0 {
		b.Service(disabledValeServiceName, disabledValeEndpoint)
		b.RouteTo(disabledValeRouteName, options.EntrypointName, disabledValeServiceName, provider.RoutePathPrefix(disabledValeRoutePath))
	}
	for i := range bucketsByUpstream {
		upstream := bucketsByUpstream[i]
		if err := addUpstreamToBuilder(b, upstream, options.EntrypointName); err != nil {
			return nil, err
		}
	}

	cfg, err := b.BuildValidated()
	if err != nil {
		return nil, oops.Wrapf(err, "build validated vale config")
	}
	return cfg, nil
}

func NewValeMemoryConfigProvider(cfg *valeconfig.Config) (*memoryconfig.Provider, error) {
	if cfg == nil {
		err := oops.New("vale config is nil")
		return nil, oops.Wrapf(err, "validate vale memory config provider")
	}
	configProvider, err := memoryconfig.New("maxio-s3-proxy", cfg)
	if err != nil {
		return nil, oops.Wrapf(err, "initialize vale memory config provider")
	}
	return configProvider, nil
}

func BuildValeGatewayFromConfig(cfg *valeconfig.Config, logger *slog.Logger) (*vale.Gateway, error) {
	if cfg == nil {
		err := oops.New("vale config is nil")
		return nil, oops.Wrapf(err, "validate vale gateway config")
	}
	opts := []vale.Option{
		vale.WithStaticConfig(cfg),
	}
	if logger != nil {
		opts = append(opts, vale.WithLogger(logger))
	}
	gateway, err := vale.New(opts...)
	if err != nil {
		return nil, oops.Wrapf(err, "initialize vale gateway")
	}
	return gateway, nil
}

func BuildValeGatewayFromProvider(
	configProvider provider.ConfigProvider,
	logger *slog.Logger,
	middlewareRegistry ...*vale.MiddlewareRegistry,
) (*vale.Gateway, error) {
	if configProvider == nil {
		err := oops.New("vale config provider is nil")
		return nil, oops.Wrapf(err, "validate vale gateway config provider")
	}
	opts := []vale.Option{
		vale.WithConfigSourceProviders(configProvider),
		vale.WithWatch(true),
	}
	if logger != nil {
		opts = append(opts, vale.WithLogger(logger))
	}
	if len(middlewareRegistry) > 0 && middlewareRegistry[0] != nil {
		opts = append(opts, vale.WithMiddlewareRegistry(middlewareRegistry[0]))
	}
	gateway, err := vale.New(opts...)
	if err != nil {
		return nil, oops.Wrapf(err, "initialize vale gateway")
	}
	return gateway, nil
}

func sortUpstreamsForDeterministicBuild(upstreams []model.Upstream) []model.Upstream {
	sorted := append([]model.Upstream(nil), upstreams...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := strings.TrimSpace(sorted[i].ID)
		if left == "" {
			left = strings.TrimSpace(sorted[i].Name)
		}
		right := strings.TrimSpace(sorted[j].ID)
		if right == "" {
			right = strings.TrimSpace(sorted[j].Name)
		}
		if left == right {
			return strings.TrimSpace(sorted[i].Name) < strings.TrimSpace(sorted[j].Name)
		}
		return left < right
	})
	return sorted
}

func addUpstreamToBuilder(b *provider.ConfigBuilder, upstream model.Upstream, entrypointName string) error {
	name := strings.TrimSpace(upstream.Name)
	if name == "" {
		name = strings.TrimSpace(upstream.ID)
	}
	if name == "" {
		err := oops.New("upstream name is required")
		return oops.Wrapf(err, "validate upstream")
	}
	endpoint := strings.TrimSpace(upstream.Endpoint)
	if endpoint == "" {
		return oops.Errorf("upstream %q endpoint is required", name)
	}
	parsedEndpoint := mo.Try(func() (*url.URL, error) {
		return url.Parse(endpoint)
	}).OrElse(nil)
	if parsedEndpoint == nil || parsedEndpoint.Scheme == "" || parsedEndpoint.Host == "" {
		return oops.Errorf("upstream %q endpoint invalid: %q", name, endpoint)
	}

	buildServiceAndRoutes(
		b,
		name,
		upstreamIdentity(upstream),
		parsedEndpoint.String(),
		upstream.Buckets,
		entrypointName,
	)
	return nil
}

func buildServiceAndRoutes(
	b *provider.ConfigBuilder,
	serviceName, upstreamID, endpointURL string,
	buckets []string,
	entrypointName string,
) {
	b.Service(serviceName, endpointURL)
	middlewareName := dedupeMiddlewareName(upstreamID)
	b.MiddlewareNamed(middlewareName, provider.MiddlewareType(dedupeMiddlewareType))

	if len(buckets) == 0 {
		b.RouteTo(
			serviceName+"-default",
			entrypointName,
			serviceName,
			provider.RoutePathPrefix("/"),
			provider.RouteMiddlewares(middlewareName),
		)
		return
	}

	candidates := slices.Clip(append([]string(nil), buckets...))
	sort.Strings(candidates)

	for _, bucket := range candidates {
		cleanBucket := strings.TrimSpace(prefixNormalized(strings.TrimSpace(bucket)))
		if cleanBucket == "" {
			continue
		}
		pathPrefix := "/" + cleanBucket + "/"
		routeName := serviceName + "-" + strings.ReplaceAll(cleanBucket, "/", "-")
		b.RouteTo(routeName, entrypointName, serviceName, provider.RoutePathPrefix(pathPrefix), provider.RouteMiddlewares(middlewareName))
	}
}

func upstreamIdentity(upstream model.Upstream) string {
	id := strings.TrimSpace(upstream.ID)
	if id != "" {
		return id
	}
	return strings.TrimSpace(upstream.Name)
}

func normalizeValeOptions(options ValeProxyBuildOptions) ValeProxyBuildOptions {
	if strings.TrimSpace(options.EntrypointName) == "" {
		options.EntrypointName = defaultValeEntrypoint
	}
	if strings.TrimSpace(options.Entrypoint) == "" {
		options.Entrypoint = defaultValeEntrypointAddress
	}
	if strings.TrimSpace(options.AdminAddress) == "" {
		options.AdminAddress = defaultValeAdminAddress
	}
	if strings.TrimSpace(options.HealthInterval) == "" {
		options.HealthInterval = defaultValeHealthInterval
	}
	if strings.TrimSpace(options.HealthTimeout) == "" {
		options.HealthTimeout = defaultValeHealthTimeout
	}
	return options
}

func prefixNormalized(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimLeft(value, "/")
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	return value
}
