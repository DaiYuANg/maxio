package processing

import (
	"context"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
)

func (cfg Config) normalized() Config {
	cfg.Mode = NormalizeMode(cfg.Mode)
	if cfg.Mode == "" {
		cfg.Mode = ModeAsyncPermissive
	}
	if !cfg.Enabled {
		cfg.Mode = ModeDisabled
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return cfg
}

func NormalizeMode(mode string) string {
	return strings.TrimSpace(strings.ToLower(mode))
}

func normalizeResult(result ProcessorResult, processorName, mode string) ProcessorResult {
	result.Processor = strings.TrimSpace(result.Processor)
	if result.Processor == "" {
		result.Processor = processorName
	}
	result.Mode = NormalizeMode(result.Mode)
	if result.Mode == "" {
		result.Mode = NormalizeMode(mode)
	}
	result.Status = NormalizeStatus(result.Status)
	if result.Status == "" {
		result.Status = StatusSucceeded
	}
	result = normalizeResultStatus(result)
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now().UTC()
	}
	return result
}

func normalizeResultStatus(result ProcessorResult) ProcessorResult {
	switch result.Status {
	case StatusSucceeded, StatusSkipped, StatusFailed, StatusBlocked:
		return result
	default:
		if strings.TrimSpace(result.Error) == "" {
			result.Error = "unknown processor status: " + result.Status
		}
		result.Status = StatusFailed
		return result
	}
}

func bindProcessors(mode string, processors ...Processor) []ProcessorBinding {
	bindings := make([]ProcessorBinding, 0, len(processors))
	for _, processor := range processors {
		bindings = append(bindings, BindProcessor(processor, mode))
	}
	return bindings
}

func normalizeProcessorBindings(defaultMode string, bindings ...ProcessorBinding) *collectionlist.List[ProcessorBinding] {
	defaultMode = NormalizeMode(defaultMode)
	if defaultMode == "" {
		defaultMode = ModeAsyncPermissive
	}
	if len(bindings) == 0 && defaultMode != ModeDisabled {
		bindings = append(bindings, BindProcessor(NewNoopProcessor(), defaultMode))
	}
	result := collectionlist.NewListWithCapacity[ProcessorBinding](len(bindings))
	for _, binding := range bindings {
		binding.Mode = NormalizeMode(binding.Mode)
		if binding.Mode == "" {
			binding.Mode = defaultMode
		}
		result.Add(binding)
	}
	return result
}

func (s *Service) bindingsForModes(modes ...string) *collectionlist.List[ProcessorBinding] {
	result := collectionlist.NewList[ProcessorBinding]()
	if s == nil || s.processors == nil {
		return result
	}
	allowed := allowedProcessingModes(modes)
	s.processors.Range(func(_ int, binding ProcessorBinding) bool {
		if binding.Processor == nil {
			return true
		}
		if _, ok := allowed[binding.Mode]; ok {
			result.Add(binding)
		}
		return true
	})
	return result
}

func allowedProcessingModes(modes []string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(modes))
	for _, mode := range modes {
		allowed[NormalizeMode(mode)] = struct{}{}
	}
	return allowed
}

func isStrictBinding(binding ProcessorBinding) bool {
	return binding.Mode == ModeInlineStrict || binding.Mode == ModeAsyncStrict
}

func hasInlineStrictBinding(bindings *collectionlist.List[ProcessorBinding]) bool {
	if bindings == nil {
		return false
	}
	found := false
	bindings.Range(func(_ int, binding ProcessorBinding) bool {
		if binding.Mode == ModeInlineStrict {
			found = true
			return false
		}
		return true
	})
	return found
}

func processorStatusResults(bindings *collectionlist.List[ProcessorBinding], status string) *collectionlist.List[ProcessorResult] {
	results := collectionlist.NewList[ProcessorResult]()
	if bindings == nil {
		return results
	}
	bindings.Range(func(_ int, binding ProcessorBinding) bool {
		if binding.Processor == nil || binding.Mode == ModeDisabled {
			return true
		}
		results.Add(ProcessorResult{Processor: binding.Processor.Name(), Mode: binding.Mode, Status: status})
		return true
	})
	return results
}

func cloneProcessorResults(results *collectionlist.List[ProcessorResult]) *collectionlist.List[ProcessorResult] {
	if results == nil {
		return collectionlist.NewList[ProcessorResult]()
	}
	return collectionlist.NewList(results.Values()...)
}

func mergeProcessorResults(base, additions *collectionlist.List[ProcessorResult]) *collectionlist.List[ProcessorResult] {
	merged := collectionlist.NewList[ProcessorResult]()
	replacements := replacementProcessorResults(additions)
	appendNonReplacedProcessorResults(merged, base, replacements)
	appendProcessorResults(merged, additions)
	return merged
}

func replacementProcessorResults(additions *collectionlist.List[ProcessorResult]) map[string]struct{} {
	replacements := map[string]struct{}{}
	appendProcessorResultKeys(replacements, additions)
	return replacements
}

func appendProcessorResultKeys(keys map[string]struct{}, results *collectionlist.List[ProcessorResult]) {
	if results == nil {
		return
	}
	results.Range(func(_ int, result ProcessorResult) bool {
		keys[processorResultKey(result)] = struct{}{}
		return true
	})
}

func appendNonReplacedProcessorResults(merged, base *collectionlist.List[ProcessorResult], replacements map[string]struct{}) {
	if base == nil {
		return
	}
	base.Range(func(_ int, result ProcessorResult) bool {
		if _, replace := replacements[processorResultKey(result)]; !replace {
			merged.Add(result)
		}
		return true
	})
}

func appendProcessorResults(merged, results *collectionlist.List[ProcessorResult]) {
	if results == nil {
		return
	}
	results.Range(func(_ int, result ProcessorResult) bool {
		merged.Add(result)
		return true
	})
}

func processorResultKey(result ProcessorResult) string {
	return strings.TrimSpace(result.Processor) + "\x00" + NormalizeMode(result.Mode)
}

type NoopProcessor struct{}

func NewNoopProcessor() NoopProcessor {
	return NoopProcessor{}
}

func (NoopProcessor) Name() string {
	return "noop"
}

func (NoopProcessor) Capabilities() *collectionset.Set[Capability] {
	return collectionset.NewSet[Capability]()
}

func (NoopProcessor) Process(context.Context, Input) (ProcessorResult, error) {
	return ProcessorResult{Processor: "noop", Status: StatusSucceeded, CompletedAt: time.Now().UTC()}, nil
}

func normalizeRecordListLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func cleanupInput(ctx context.Context, input Input) {
	if input.Cleanup != nil {
		input.Cleanup(ctx)
	}
}
