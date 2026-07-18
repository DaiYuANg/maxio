package processing

import (
	"context"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/samber/lo"
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
	return lo.Map(processors, func(processor Processor, _ int) ProcessorBinding {
		return BindProcessor(processor, mode)
	})
}

func normalizeProcessorBindings(defaultMode string, bindings ...ProcessorBinding) *collectionlist.List[ProcessorBinding] {
	defaultMode = NormalizeMode(defaultMode)
	if defaultMode == "" {
		defaultMode = ModeAsyncPermissive
	}
	if len(bindings) == 0 && defaultMode != ModeDisabled {
		bindings = append(bindings, BindProcessor(NewNoopProcessor(), defaultMode))
	}
	return collectionlist.NewList(lo.Map(bindings, func(binding ProcessorBinding, _ int) ProcessorBinding {
		binding.Mode = NormalizeMode(binding.Mode)
		if binding.Mode == "" {
			binding.Mode = defaultMode
		}
		return binding
	})...)
}

func (s *Service) bindingsForModes(modes ...string) *collectionlist.List[ProcessorBinding] {
	if s == nil {
		return collectionlist.NewList[ProcessorBinding]()
	}
	return filterProcessorBindings(s.processors, modes...)
}

func filterProcessorBindings(
	bindings *collectionlist.List[ProcessorBinding],
	modes ...string,
) *collectionlist.List[ProcessorBinding] {
	if bindings == nil {
		return collectionlist.NewList[ProcessorBinding]()
	}
	allowed := allowedProcessingModes(modes)
	return collectionlist.FilterMapList(bindings, func(_ int, binding ProcessorBinding) (ProcessorBinding, bool) {
		if binding.Processor == nil {
			return ProcessorBinding{}, false
		}
		_, ok := allowed[binding.Mode]
		return binding, ok
	})
}

func (s *Service) inlineStrictBindings() *collectionlist.List[ProcessorBinding] {
	if s == nil || s.inlineBindings == nil {
		return collectionlist.NewList[ProcessorBinding]()
	}
	return s.inlineBindings
}

func (s *Service) asyncProcessorBindings() *collectionlist.List[ProcessorBinding] {
	if s == nil || s.asyncBindings == nil {
		return collectionlist.NewList[ProcessorBinding]()
	}
	return s.asyncBindings
}

func (s *Service) strictReadBindings() *collectionlist.List[ProcessorBinding] {
	if s == nil || s.strictBindings == nil {
		return collectionlist.NewList[ProcessorBinding]()
	}
	return s.strictBindings
}

func allowedProcessingModes(modes []string) map[string]struct{} {
	return lo.SliceToMap(modes, func(mode string) (string, struct{}) {
		return NormalizeMode(mode), struct{}{}
	})
}

func isStrictBinding(binding ProcessorBinding) bool {
	return binding.Mode == ModeInlineStrict || binding.Mode == ModeAsyncStrict
}

func hasInlineStrictBinding(bindings *collectionlist.List[ProcessorBinding]) bool {
	if bindings == nil {
		return false
	}
	return lo.ContainsBy(bindings.Values(), func(binding ProcessorBinding) bool {
		return binding.Mode == ModeInlineStrict
	})
}

func processorStatusResults(bindings *collectionlist.List[ProcessorBinding], status string) *collectionlist.List[ProcessorResult] {
	if bindings == nil {
		return collectionlist.NewList[ProcessorResult]()
	}
	return collectionlist.FilterMapList(bindings, func(_ int, binding ProcessorBinding) (ProcessorResult, bool) {
		if binding.Processor == nil || binding.Mode == ModeDisabled {
			return ProcessorResult{}, false
		}
		return ProcessorResult{Processor: binding.Processor.Name(), Mode: binding.Mode, Status: status}, true
	})
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
