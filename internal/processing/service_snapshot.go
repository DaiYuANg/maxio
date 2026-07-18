package processing

import (
	"sort"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/samber/lo"
)

func (s *Service) Snapshot() Snapshot {
	if s == nil || !s.cfg.Enabled {
		return disabledSnapshot()
	}
	builder := newSnapshotBuilder()
	if s.processors != nil {
		s.processors.Range(func(_ int, binding ProcessorBinding) bool {
			builder.add(binding)
			return true
		})
	}
	return builder.snapshot(s.cfg)
}

func disabledSnapshot() Snapshot {
	return Snapshot{
		Mode:              ModeDisabled,
		Processors:        collectionlist.NewList[string](),
		ProcessorModes:    map[string]string{},
		ProcessorFailOpen: map[string]bool{},
		Capabilities:      collectionlist.NewList[string](),
	}
}

type snapshotBuilder struct {
	names             *collectionlist.List[string]
	capabilities      *collectionset.Set[string]
	processorModes    map[string]string
	processorFailOpen map[string]bool
}

func newSnapshotBuilder() snapshotBuilder {
	return snapshotBuilder{
		names:             collectionlist.NewList[string](),
		capabilities:      collectionset.NewSet[string](),
		processorModes:    map[string]string{},
		processorFailOpen: map[string]bool{},
	}
}

func (b snapshotBuilder) add(binding ProcessorBinding) {
	processor := binding.Processor
	if processor == nil || binding.Mode == ModeDisabled {
		return
	}
	name := strings.TrimSpace(processor.Name())
	if name != "" {
		b.names.Add(name)
		b.processorModes[name] = binding.Mode
		if provider, ok := processor.(ProcessorFailOpenProvider); ok {
			b.processorFailOpen[name] = provider.FailOpen()
		}
	}
	b.addCapabilities(processor.Capabilities())
}

func (b snapshotBuilder) addCapabilities(processorCapabilities *collectionset.Set[Capability]) {
	if processorCapabilities == nil {
		return
	}
	lo.ForEach(
		lo.FilterMap(processorCapabilities.Values(), func(capability Capability, _ int) (string, bool) {
			capabilityValue := strings.TrimSpace(string(capability))
			return capabilityValue, capabilityValue != ""
		}),
		func(capability string, _ int) {
			b.capabilities.Add(capability)
		},
	)
}

func (b snapshotBuilder) snapshot(cfg Config) Snapshot {
	capabilityValues := b.capabilities.Values()
	sort.Strings(capabilityValues)
	return Snapshot{
		Enabled:           cfg.Enabled,
		Mode:              cfg.Mode,
		FailOpen:          cfg.FailOpen,
		Timeout:           cfg.Timeout,
		Processors:        b.names,
		ProcessorModes:    b.processorModes,
		ProcessorFailOpen: b.processorFailOpen,
		Capabilities:      collectionlist.NewList(capabilityValues...),
	}
}
