package proxy

import (
	"log/slog"
	"net/url"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
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

func BuildValeConfigFromUpstreams(upstreams *collectionlist.List[model.Upstream], options ValeProxyBuildOptions) (*valeconfig.Config, error) {
	if upstreams == nil || upstreams.Len() == 0 {
		err := oops.New("vale upstream list is empty")
		return nil, oops.Wrapf(err, "validate vale upstreams")
	}
	return BuildValeConfigSnapshot(upstreams, options)
}

func BuildValeConfigSnapshot(upstreams *collectionlist.List[model.Upstream], options ValeProxyBuildOptions) (*valeconfig.Config, error) {
	options = normalizeValeOptions(options)

	b := provider.NewConfigBuilder()
	b.Entrypoint(options.EntrypointName, options.Entrypoint).
		Admin(options.AdminAddress).
		Observability(options.EnableAccessLog, options.EnableMetrics)
	if options.EnableHealthCheck {
		b.Health(options.HealthInterval, options.HealthTimeout)
	}

	bucketsByUpstream := sortUpstreamsForDeterministicBuild(upstreams)
	if bucketsByUpstream == nil || bucketsByUpstream.Len() == 0 {
		b.Service(disabledValeServiceName, disabledValeEndpoint)
		b.RouteTo(disabledValeRouteName, options.EntrypointName, disabledValeServiceName, provider.RoutePathPrefix(disabledValeRoutePath))
	}
	var buildErr error
	bucketsByUpstream.Range(func(_ int, upstream model.Upstream) bool {
		if buildErr != nil {
			return false
		}
		buildErr = addUpstreamToBuilder(b, upstream, options.EntrypointName)
		return buildErr == nil
	})
	if buildErr != nil {
		return nil, buildErr
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

func sortUpstreamsForDeterministicBuild(upstreams *collectionlist.List[model.Upstream]) *collectionlist.List[model.Upstream] {
	if upstreams == nil || upstreams.Len() == 0 {
		return collectionlist.NewList[model.Upstream]()
	}
	sorted := upstreams.Sort(func(left, right model.Upstream) int {
		leftID := strings.TrimSpace(left.ID)
		if leftID == "" {
			leftID = strings.TrimSpace(left.Name)
		}
		rightID := strings.TrimSpace(right.ID)
		if rightID == "" {
			rightID = strings.TrimSpace(right.Name)
		}
		if leftID == rightID {
			return strings.Compare(strings.TrimSpace(left.Name), strings.TrimSpace(right.Name))
		}
		return strings.Compare(leftID, rightID)
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

	normalizedBuckets := collectionlist.FilterMapList(
		collectionlist.NewList(buckets...),
		func(_ int, bucket string) (string, bool) {
			cleanBucket := normalizeUpstreamBucket(bucket)
			if cleanBucket == "" {
				return "", false
			}
			return cleanBucket, true
		},
	)
	if normalizedBuckets == nil || normalizedBuckets.Len() == 0 {
		b.RouteTo(
			serviceName+"-default",
			entrypointName,
			serviceName,
			provider.RoutePathPrefix("/"),
			provider.RouteMiddlewares(middlewareName),
		)
		return
	}

	candidates := uniqueSortedBuckets(normalizedBuckets)
	candidates.Range(func(_ int, cleanBucket string) bool {
		pathPrefix := "/" + cleanBucket + "/"
		routeName := serviceName + "-" + strings.ReplaceAll(cleanBucket, "/", "-")
		b.RouteTo(routeName, entrypointName, serviceName, provider.RoutePathPrefix(pathPrefix), provider.RouteMiddlewares(middlewareName))
		return true
	})
}

func uniqueSortedBuckets(buckets *collectionlist.List[string]) *collectionlist.List[string] {
	if buckets == nil || buckets.Len() == 0 {
		return collectionlist.NewList[string]()
	}
	sortedBuckets := buckets.Sort(strings.Compare)
	uniqueBuckets := collectionset.NewOrderedSetWithCapacity[string](sortedBuckets.Len(), sortedBuckets.Values()...)
	return collectionlist.NewList(uniqueBuckets.Values()...)
}

func normalizeUpstreamBucket(value string) string {
	return prefixNormalized(strings.TrimSpace(value))
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
