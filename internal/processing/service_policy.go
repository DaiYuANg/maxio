package processing

import (
	"errors"
	"fmt"
	"strings"

	collectionlist "github.com/arcgolabs/collectionx/list"
)

func (s *Service) strictError(err error) error {
	if s == nil || s.cfg.FailOpen {
		return nil
	}
	return err
}

func (s *Service) hasStrictReadGate() bool {
	return s.bindingsForModes(ModeInlineStrict, ModeAsyncStrict).Len() > 0
}

func (s *Service) strictRecordError(record Record) error {
	strictBindings := s.bindingsForModes(ModeInlineStrict, ModeAsyncStrict)
	if strictBindings.Len() == 0 {
		return nil
	}
	results := processorResultsByKey(record.Results)
	var gateErr error
	strictBindings.Range(func(_ int, binding ProcessorBinding) bool {
		if binding.Processor == nil {
			return true
		}
		result, found := lookupProcessorResult(results, binding.Processor.Name(), binding.Mode, record.Mode)
		if !found {
			gateErr = ErrProcessingPending
			return false
		}
		if err := processingStatusError(result.Status, result.Error); err != nil {
			gateErr = err
			return false
		}
		return true
	})
	return s.strictError(gateErr)
}

func processingReadBlockReason(err error) string {
	switch {
	case errors.Is(err, ErrProcessingPending):
		return "pending"
	case errors.Is(err, ErrProcessingDenied):
		return "denied"
	case errors.Is(err, ErrProcessingFailed):
		return "failed"
	default:
		return "error"
	}
}

func processorResultsByKey(results *collectionlist.List[ProcessorResult]) map[string]ProcessorResult {
	byKey := map[string]ProcessorResult{}
	if results == nil {
		return byKey
	}
	results.Range(func(_ int, result ProcessorResult) bool {
		byKey[processorResultKey(result)] = result
		return true
	})
	return byKey
}

func lookupProcessorResult(results map[string]ProcessorResult, processorName, mode, recordMode string) (ProcessorResult, bool) {
	processorName = strings.TrimSpace(processorName)
	mode = NormalizeMode(mode)
	if result, found := results[processorName+"\x00"+mode]; found {
		return result, true
	}
	recordMode = NormalizeMode(recordMode)
	if recordMode == mode {
		if result, found := results[processorName+"\x00"]; found {
			return result, true
		}
	}
	return ProcessorResult{}, false
}

func processingStatusError(status, errorText string) error {
	switch NormalizeStatus(status) {
	case StatusSucceeded:
		return nil
	case StatusSkipped:
		return skippedStatusError(errorText)
	case StatusQueued, StatusRunning:
		return ErrProcessingPending
	case StatusBlocked:
		return ErrProcessingDenied
	case StatusFailed:
		return fmt.Errorf("%w: %s", ErrProcessingFailed, errorText)
	default:
		return ErrProcessingPending
	}
}

func skippedStatusError(errorText string) error {
	if strings.TrimSpace(errorText) == "" {
		errorText = "strict processor skipped"
	}
	return fmt.Errorf("%w: %s", ErrProcessingFailed, errorText)
}
