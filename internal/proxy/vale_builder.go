package proxy

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/arcgolabs/vale"
	valeconfig "github.com/arcgolabs/vale/config"
	"github.com/arcgolabs/vale/provider"
	"github.com/lyonbrown4d/maxio/model"
)

const defaultValeEntrypoint = "web"
const defaultValeEntrypointAddress = ":8080"
const defaultValeAdminAddress = ":19090"
const defaultValeHealthInterval = "5s"
const defaultValeHealthTimeout = "2s"

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
	options = normalizeValeOptions(options)
	if len(upstreams) == 0 {
		return nil, errors.New("vale upstream list is empty")
	}

	b := provider.NewConfigBuilder()
	b.Entrypoint(options.EntrypointName, options.Entrypoint).
		Admin(options.AdminAddress).
		Observability(options.EnableAccessLog, options.EnableMetrics)
	if options.EnableHealthCheck {
		b.Health(options.HealthInterval, options.HealthTimeout)
	}

	bucketsByUpstream := sortUpstreamsForDeterministicBuild(upstreams)
	for _, upstream := range bucketsByUpstream {
		if err := addUpstreamToBuilder(b, upstream, options.EntrypointName); err != nil {
			return nil, err
		}
	}

	cfg, err := b.BuildValidated()
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func BuildValeGatewayFromConfig(cfg *valeconfig.Config, logger *slog.Logger) (*vale.Gateway, error) {
	if cfg == nil {
		return nil, errors.New("vale config is nil")
	}
	opts := []vale.Option{
		vale.WithStaticConfig(cfg),
	}
	if logger != nil {
		opts = append(opts, vale.WithLogger(logger))
	}
	return vale.New(opts...)
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
		return fmt.Errorf("upstream name is required")
	}
	endpoint := strings.TrimSpace(upstream.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("upstream %q endpoint is required", name)
	}
	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil || parsedEndpoint.Scheme == "" || parsedEndpoint.Host == "" {
		return fmt.Errorf("upstream %q endpoint invalid: %q", name, endpoint)
	}

	if err := buildServiceAndRoutes(b, name, parsedEndpoint.String(), upstream.Buckets, entrypointName); err != nil {
		return fmt.Errorf("upstream %q config: %w", name, err)
	}
	return nil
}

func buildServiceAndRoutes(
	b *provider.ConfigBuilder,
	serviceName, endpointURL string,
	buckets []string,
	entrypointName string,
) error {
	b.Service(serviceName, endpointURL)

	if len(buckets) == 0 {
		b.RouteTo(serviceName+"-default", entrypointName, serviceName, provider.RoutePathPrefix("/"))
		return nil
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
		b.RouteTo(routeName, entrypointName, serviceName, provider.RoutePathPrefix(pathPrefix))
	}
	return nil
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
