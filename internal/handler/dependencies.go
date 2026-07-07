package handler

import (
	"context"

	"github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/indexcontrol"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/processing"
)

type proxyReloader interface {
	Reload(ctx context.Context) error
}

// Dependencies groups handler dependencies to keep dix providers shallow.
type Dependencies struct {
	metadata     metadata.MetadataStore
	search       *index.SearchEngine
	indexManager *indexcontrol.Manager
	processing   *processing.Service
	proxy        proxyReloader
}

// NewDependencies wires the handler dependency set.
func NewDependencies(
	metadataStore metadata.MetadataStore,
	proxyRuntime ...proxyReloader,
) Dependencies {
	return newDependencies(metadataStore, nil, proxyRuntime...)
}

func newDependencies(
	metadataStore metadata.MetadataStore,
	searchEngine *index.SearchEngine,
	proxyRuntime ...proxyReloader,
) Dependencies {
	return newDependenciesWithProcessing(metadataStore, searchEngine, nil, proxyRuntime...)
}

func newDependenciesWithProcessing(
	metadataStore metadata.MetadataStore,
	searchEngine *index.SearchEngine,
	processor *processing.Service,
	proxyRuntime ...proxyReloader,
) Dependencies {
	var reloader proxyReloader
	if len(proxyRuntime) > 0 {
		reloader = proxyRuntime[0]
	}
	return Dependencies{
		metadata:     metadataStore,
		search:       searchEngine,
		indexManager: indexcontrol.NewManager(metadataStore, searchEngine, indexcontrol.ManagerOptions{}),
		processing:   processor,
		proxy:        reloader,
	}
}
