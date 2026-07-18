package processing

import (
	"context"
	"log/slog"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/metadata"
)

func Module() dix.Module {
	return dix.NewModule(
		"processing",
		dix.WithModuleProviders(
			dix.Provider3(newServiceFromRuntimeConfig),
		),
		dix.Hooks(
			dix.OnStop(closeService),
		),
	)
}

func newServiceFromRuntimeConfig(cfg config.Config, logger *slog.Logger, store metadata.MetadataStore) *Service {
	return NewServiceWithBindings(logger, Config{
		Enabled:  cfg.ProcessingEnabled,
		Mode:     cfg.ProcessingMode,
		Timeout:  cfg.ProcessingTimeoutDuration(),
		FailOpen: cfg.ProcessingFailOpen,
	}, store, processorBindingsFromRuntimeConfig(cfg)...)
}

func closeService(ctx context.Context, service *Service) error {
	if service == nil {
		return nil
	}
	return service.Close(ctx)
}

func processorBindingsFromRuntimeConfig(cfg config.Config) []ProcessorBinding {
	processors := make([]ProcessorBinding, 0, 2)
	if cfg.ProcessingClamAVEnabled {
		processors = append(processors, BindProcessor(NewClamAVProcessor(ClamAVConfig{
			Address: cfg.ProcessingClamAVAddress,
			Timeout: cfg.ProcessingTimeoutDuration(),
		}), cfg.ProcessingClamAVMode))
	}
	if cfg.ProcessingTikaEnabled {
		processors = append(processors, BindProcessor(NewTikaProcessor(TikaConfig{
			URL:      cfg.ProcessingTikaURL,
			Timeout:  cfg.ProcessingTimeoutDuration(),
			MaxBytes: cfg.ProcessingTikaMaxBytes,
			FailOpen: cfg.ProcessingTikaFailOpen,
		}), cfg.ProcessingTikaMode))
	}
	if cfg.ProcessingEnabled && len(processors) == 0 {
		processors = append(processors, BindProcessor(NewNoopProcessor(), cfg.ProcessingMode))
	}
	return processors
}
