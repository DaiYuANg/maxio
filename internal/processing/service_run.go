package processing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func (s *Service) run(ctx context.Context, input Input, bindings *collectionlist.List[ProcessorBinding], base *collectionlist.List[ProcessorResult]) error {
	if bindings == nil || bindings.Len() == 0 {
		return s.storeSkippedRecord(ctx, input, bindings, base)
	}
	runCtx, cancel := s.runContext(ctx)
	defer cancel()
	if err := s.storeRunningRecord(runCtx, input, bindings, base); err != nil {
		return err
	}
	results, runErr := s.runProcessors(runCtx, input, bindings)
	return s.finishRun(runCtx, input, bindings, mergeProcessorResults(base, results), runErr)
}

func (s *Service) storeSkippedRecord(ctx context.Context, input Input, bindings *collectionlist.List[ProcessorBinding], base *collectionlist.List[ProcessorResult]) error {
	if base != nil && base.Len() > 0 {
		return nil
	}
	if err := s.storeRecord(ctx, input.Object, StatusSkipped, "", collectionlist.NewList[ProcessorResult]()); err != nil {
		return s.bindingStoreRecordError(err, bindings)
	}
	return nil
}

func (s *Service) runContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.cfg.Timeout > 0 {
		return context.WithTimeout(ctx, s.cfg.Timeout)
	}
	return ctx, func() {}
}

func (s *Service) storeRunningRecord(ctx context.Context, input Input, bindings *collectionlist.List[ProcessorBinding], base *collectionlist.List[ProcessorResult]) error {
	running := mergeProcessorResults(base, processorStatusResults(bindings, StatusRunning))
	if err := s.storeRecord(ctx, input.Object, statusFromResults(running), "", running); err != nil {
		return s.bindingStoreRecordError(err, bindings)
	}
	return nil
}

func (s *Service) runProcessors(ctx context.Context, input Input, bindings *collectionlist.List[ProcessorBinding]) (*collectionlist.List[ProcessorResult], error) {
	results := collectionlist.NewList[ProcessorResult]()
	continueAfterProcessorError := !hasInlineStrictBinding(bindings)
	var runErr error
	bindings.Range(func(_ int, binding ProcessorBinding) bool {
		result, shouldRun, resultErr := s.runProcessor(ctx, input, binding)
		if !shouldRun {
			return true
		}
		results.Add(result)
		runErr = errors.Join(runErr, resultErr)
		return continueAfterProcessorError || runErr == nil
	})
	return results, runErr
}

func (s *Service) runProcessor(ctx context.Context, input Input, binding ProcessorBinding) (ProcessorResult, bool, error) {
	processor := binding.Processor
	if processor == nil || binding.Mode == ModeDisabled {
		return ProcessorResult{}, false, nil
	}
	result, err := processor.Process(ctx, input)
	result = normalizeResult(result, processor.Name(), binding.Mode)
	resultErr := processorResultError(&result, binding, err)
	return result, true, resultErr
}

func processorResultError(result *ProcessorResult, binding ProcessorBinding, processErr error) error {
	var resultErr error
	if processErr != nil {
		if result.Status == StatusSucceeded {
			result.Status = StatusFailed
		}
		result.Error = processErr.Error()
		resultErr = errors.Join(resultErr, processErr)
	}
	if result.Status == StatusSkipped && isStrictBinding(binding) {
		resultErr = errors.Join(resultErr, strictSkippedProcessorError(result))
	}
	if result.Status == StatusFailed {
		resultErr = errors.Join(resultErr, fmt.Errorf("%w: %s", ErrProcessingFailed, result.Error))
	}
	if result.Status == StatusBlocked {
		resultErr = errors.Join(resultErr, ErrProcessingDenied)
	}
	return resultErr
}

func strictSkippedProcessorError(result *ProcessorResult) error {
	if strings.TrimSpace(result.Error) == "" {
		result.Error = "strict processor skipped"
	}
	return fmt.Errorf("%w: %s", ErrProcessingFailed, result.Error)
}

func (s *Service) finishRun(ctx context.Context, input Input, bindings *collectionlist.List[ProcessorBinding], finalResults *collectionlist.List[ProcessorResult], runErr error) error {
	if runErr != nil {
		if err := s.storeRecord(ctx, input.Object, statusFromError(runErr), runErr.Error(), finalResults); err != nil && hasInlineStrictBinding(bindings) {
			runErr = errors.Join(runErr, fmt.Errorf("store processing record: %w", err))
		}
		return s.strictError(runErr)
	}
	if err := s.storeRecord(ctx, input.Object, statusFromResults(finalResults), "", finalResults); err != nil {
		return s.bindingStoreRecordError(err, bindings)
	}
	return nil
}

func (s *Service) bindingStoreRecordError(err error, bindings *collectionlist.List[ProcessorBinding]) error {
	if err == nil || s == nil || !hasInlineStrictBinding(bindings) {
		return nil
	}
	return s.strictError(fmt.Errorf("store processing record: %w", err))
}

func statusFromError(err error) string {
	if errors.Is(err, ErrProcessingDenied) {
		return StatusBlocked
	}
	return StatusFailed
}

func statusFromResults(results *collectionlist.List[ProcessorResult]) string {
	if results == nil || results.Len() == 0 {
		return StatusSkipped
	}
	status := StatusSkipped
	results.Range(func(_ int, result ProcessorResult) bool {
		status = higherPriorityStatus(status, result.Status)
		return status != StatusBlocked && status != StatusFailed
	})
	return status
}

func higherPriorityStatus(current, candidate string) string {
	if processingStatusPriority(candidate) > processingStatusPriority(current) {
		return candidate
	}
	return current
}

func processingStatusPriority(status string) int {
	switch status {
	case StatusBlocked:
		return 6
	case StatusFailed:
		return 5
	case StatusRunning:
		return 4
	case StatusQueued:
		return 3
	case StatusSucceeded:
		return 2
	case StatusSkipped:
		return 1
	default:
		return 0
	}
}
