package handler

import (
	"context"

	"github.com/lyonbrown4d/maxio/internal/discovery"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/object"
)

type proxyReloader interface {
	Reload(ctx context.Context) error
}

// Dependencies groups handler dependencies to keep dix providers shallow.
type Dependencies struct {
	objects   *object.Service
	discovery *discovery.Runtime
	metadata  metadata.MetadataStore
	proxy     proxyReloader
}

// NewDependencies wires the handler dependency set.
func NewDependencies(
	discoveryRuntime *discovery.Runtime,
	metadataStore metadata.MetadataStore,
	proxyRuntime ...proxyReloader,
) Dependencies {
	var reloader proxyReloader
	if len(proxyRuntime) > 0 {
		reloader = proxyRuntime[0]
	}
	return Dependencies{discovery: discoveryRuntime, metadata: metadataStore, proxy: reloader}
}
