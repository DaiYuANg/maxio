package processing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const (
	defaultTimeout         = 30 * time.Second
	maxDiscardedTombstones = 4096
)

type Service struct {
	logger         *slog.Logger
	cfg            Config
	store          RecordStore
	processors     *collectionlist.List[ProcessorBinding]
	mu             sync.RWMutex
	records        map[string]Record
	discarded      map[string]struct{}
	discardedOrder []string
}

func NewService(logger *slog.Logger, cfg Config, processors ...Processor) *Service {
	return NewServiceWithStore(logger, cfg, nil, processors...)
}

func NewServiceWithStore(logger *slog.Logger, cfg Config, store RecordStore, processors ...Processor) *Service {
	cfg = cfg.normalized()
	return NewServiceWithBindings(logger, cfg, store, bindProcessors(cfg.Mode, processors...)...)
}

func NewServiceWithBindings(logger *slog.Logger, cfg Config, store RecordStore, processors ...ProcessorBinding) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	cfg = cfg.normalized()
	return &Service{
		logger:         logger,
		cfg:            cfg,
		store:          store,
		processors:     normalizeProcessorBindings(cfg.Mode, processors...),
		records:        make(map[string]Record),
		discarded:      make(map[string]struct{}),
		discardedOrder: make([]string, 0),
	}
}

func (s *Service) Snapshot() Snapshot {
	if s == nil || !s.cfg.Enabled {
		return Snapshot{Mode: ModeDisabled, Processors: collectionlist.NewList[string](), ProcessorModes: map[string]string{}, ProcessorFailOpen: map[string]bool{}, Capabilities: collectionlist.NewList[string]()}
	}
	names := collectionlist.NewList[string]()
	capabilities := collectionset.NewSet[string]()
	processorModes := map[string]string{}
	processorFailOpen := map[string]bool{}
	if s.processors != nil {
		s.processors.Range(func(_ int, binding ProcessorBinding) bool {
			processor := binding.Processor
			if processor == nil || binding.Mode == ModeDisabled {
				return true
			}
			if name := strings.TrimSpace(processor.Name()); name != "" {
				names.Add(name)
				processorModes[name] = binding.Mode
				if provider, ok := processor.(ProcessorFailOpenProvider); ok {
					processorFailOpen[name] = provider.FailOpen()
				}
			}
			processorCapabilities := processor.Capabilities()
			if processorCapabilities != nil {
				for _, capability := range processorCapabilities.Values() {
					if value := strings.TrimSpace(string(capability)); value != "" {
						capabilities.Add(value)
					}
				}
			}
			return true
		})
	}
	capabilityValues := capabilities.Values()
	sort.Strings(capabilityValues)
	return Snapshot{
		Enabled:           s.cfg.Enabled,
		Mode:              s.cfg.Mode,
		FailOpen:          s.cfg.FailOpen,
		Timeout:           s.cfg.Timeout,
		Processors:        names,
		ProcessorModes:    processorModes,
		ProcessorFailOpen: processorFailOpen,
		Capabilities:      collectionlist.NewList(capabilityValues...),
	}
}

func (s *Service) ProcessBeforeCommit(ctx context.Context, input Input) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	bindings := s.bindingsForModes(ModeInlineStrict)
	if bindings.Len() == 0 {
		return nil
	}
	return s.run(contextOrBackground(ctx), input, bindings, nil)
}

func (s *Service) ProcessAfterCommit(ctx context.Context, input Input) {
	if s == nil || !s.cfg.Enabled {
		return
	}
	ctx = contextOrBackground(ctx)
	recordExists := false
	base := collectionlist.NewList[ProcessorResult]()
	if record, found := s.Record(ctx, input.Object); found {
		recordExists = true
		base = cloneProcessorResults(record.Results)
	} else if record, digestFound := s.promotableDigestRecord(ctx, input.Object); digestFound {
		recordExists = true
		base = cloneProcessorResults(record.Results)
		_ = s.storeRecord(ctx, input.Object, record.Status, record.Error, base)
		s.discardDigestRecord(ctx, input.Object)
	}

	asyncBindings := s.bindingsForModes(ModeAsyncPermissive, ModeAsyncStrict)
	if asyncBindings.Len() > 0 {
		queued := mergeProcessorResults(base, processorStatusResults(asyncBindings, StatusQueued))
		_ = s.storeRecord(ctx, input.Object, statusFromResults(queued), "", queued)
		go func(base *collectionlist.List[ProcessorResult]) {
			runCtx := context.WithoutCancel(ctx)
			defer cleanupInput(runCtx, input)
			if err := s.run(runCtx, input, asyncBindings, base); err != nil && s.logger != nil {
				s.logger.WarnContext(runCtx, "object post-commit processing failed", "bucket", input.Object.Bucket, "key", input.Object.Key, "version_id", input.Object.VersionID, "error", err)
			}
		}(cloneProcessorResults(base))
		return
	}

	if recordExists {
		return
	}
	_ = s.storeRecord(ctx, input.Object, StatusSucceeded, "", collectionlist.NewList[ProcessorResult]())
}

func (s *Service) EnsureReadAllowed(ctx context.Context, object ObjectRef) error {
	if s == nil || !s.cfg.Enabled || !s.hasStrictReadGate() {
		return nil
	}
	record, found, err := s.lookupRecord(contextOrBackground(ctx), object)
	if err != nil {
		return s.strictError(fmt.Errorf("object processing record lookup: %w", err))
	}
	if !found {
		return s.strictError(ErrProcessingPending)
	}
	return s.strictRecordError(record)
}

func (s *Service) ReadDecision(record Record) ReadDecision {
	if s == nil || !s.cfg.Enabled || !s.hasStrictReadGate() {
		return ReadDecision{Allowed: true}
	}
	err := s.strictRecordError(record)
	if err == nil {
		return ReadDecision{Allowed: true}
	}
	return ReadDecision{Allowed: false, Reason: processingReadBlockReason(err)}
}

func (s *Service) Discard(ctx context.Context, object ObjectRef) {
	if s == nil {
		return
	}
	ctx = contextOrBackground(ctx)
	if object.VersionID != "" {
		s.discardVersion(ctx, object)
		return
	}
	s.discardDigest(ctx, object)
}

func (s *Service) discardVersion(ctx context.Context, object ObjectRef) {
	key := objectKey(object)
	s.mu.Lock()
	s.rememberDiscardedLocked(key)
	delete(s.records, key)
	s.mu.Unlock()
	s.deleteRecordFromStore(ctx, object)
}

func (s *Service) discardDigest(ctx context.Context, object ObjectRef) {
	key := objectKey(object)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleteRecordFromStore(ctx, object)
	delete(s.records, key)
}

func (s *Service) deleteRecordFromStore(ctx context.Context, object ObjectRef) {
	if s.store == nil {
		return
	}
	if _, err := s.store.DeleteProcessingRecord(ctx, object.Bucket, object.Key, object.VersionID, object.Digest); err != nil && s.logger != nil {
		s.logger.WarnContext(ctx, "delete processing record", "bucket", object.Bucket, "key", object.Key, "version_id", object.VersionID, "error", err)
	}
}

func (s *Service) Record(ctx context.Context, object ObjectRef) (Record, bool) {
	record, found, err := s.LookupRecord(ctx, object)
	if err != nil {
		return Record{}, false
	}
	return record, found
}

func (s *Service) LookupRecord(ctx context.Context, object ObjectRef) (Record, bool, error) {
	return s.lookupRecord(ctx, object)
}

func (s *Service) ListRecords(ctx context.Context, status string, limit int) (*collectionlist.List[Record], error) {
	if s == nil {
		return collectionlist.NewList[Record](), nil
	}
	ctx = contextOrBackground(ctx)
	status = NormalizeStatus(status)
	limit = normalizeRecordListLimit(limit)
	if s.store != nil {
		stored, err := s.store.ListProcessingRecords(ctx, status, limit+s.discardedTombstoneCount())
		if err != nil {
			if s.logger != nil {
				s.logger.WarnContext(ctx, "list processing records", "status", status, "limit", limit, "error", err)
			}
			return nil, err
		}
		if stored == nil {
			return collectionlist.NewList[Record](), nil
		}
		records := collectionlist.NewListWithCapacity[Record](stored.Len())
		stored.Range(func(_ int, storedRecord model.ProcessingRecord) bool {
			record := recordFromModel(storedRecord)
			if !s.isDiscarded(record.Object) {
				records.Add(record)
			}
			return records.Len() < limit
		})
		return records, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	records := collectionlist.NewListWithCapacity[Record](len(s.records))
	for _, record := range s.records {
		if status == "" || record.Status == status {
			records.Add(record)
		}
	}
	sorted := records.Sort(func(left, right Record) int {
		if left.UpdatedAt.After(right.UpdatedAt) {
			return -1
		}
		if left.UpdatedAt.Before(right.UpdatedAt) {
			return 1
		}
		return strings.Compare(objectKey(left.Object), objectKey(right.Object))
	})
	if sorted.Len() <= limit {
		return sorted, nil
	}
	return collectionlist.NewList(sorted.Values()[:limit]...), nil
}

func (s *Service) lookupRecord(ctx context.Context, object ObjectRef) (Record, bool, error) {
	if s == nil {
		return Record{}, false, nil
	}
	ctx = contextOrBackground(ctx)
	if s.isDiscarded(object) {
		return Record{}, false, nil
	}
	if s.store != nil {
		stored, found, err := s.store.GetProcessingRecord(ctx, object.Bucket, object.Key, object.VersionID, object.Digest)
		if s.isDiscarded(object) {
			return Record{}, false, nil
		}
		if err != nil {
			if s.logger != nil {
				s.logger.WarnContext(ctx, "get processing record", "bucket", object.Bucket, "key", object.Key, "version_id", object.VersionID, "error", err)
			}
			return Record{}, false, err
		}
		if found {
			return recordFromModel(stored), true, nil
		}
		return Record{}, false, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, found := s.records[objectKey(object)]
	return record, found, nil
}

func (s *Service) discardDigestRecord(ctx context.Context, object ObjectRef) {
	if object.Digest == "" {
		return
	}
	digestObject := object
	digestObject.VersionID = ""
	s.Discard(ctx, digestObject)
}

func (s *Service) promotableDigestRecord(ctx context.Context, object ObjectRef) (Record, bool) {
	if object.VersionID == "" || object.Digest == "" {
		return Record{}, false
	}
	digestObject := object
	digestObject.VersionID = ""
	record, found, err := s.lookupRecord(ctx, digestObject)
	if err != nil {
		if s.logger != nil {
			s.logger.WarnContext(contextOrBackground(ctx), "get digest processing record", "bucket", object.Bucket, "key", object.Key, "digest", object.Digest, "error", err)
		}
		return Record{}, false
	}
	return record, found
}

func (s *Service) run(ctx context.Context, input Input, bindings *collectionlist.List[ProcessorBinding], base *collectionlist.List[ProcessorResult]) error {
	if bindings == nil || bindings.Len() == 0 {
		if base == nil || base.Len() == 0 {
			if err := s.storeRecord(ctx, input.Object, StatusSkipped, "", collectionlist.NewList[ProcessorResult]()); err != nil {
				return s.bindingStoreRecordError(err, bindings)
			}
		}
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx := ctx
	cancel := func() {}
	if s.cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, s.cfg.Timeout)
	}
	defer cancel()

	running := mergeProcessorResults(base, processorStatusResults(bindings, StatusRunning))
	if err := s.storeRecord(runCtx, input.Object, statusFromResults(running), "", running); err != nil {
		return s.bindingStoreRecordError(err, bindings)
	}

	results := collectionlist.NewList[ProcessorResult]()
	continueAfterProcessorError := !hasInlineStrictBinding(bindings)
	var runErr error
	bindings.Range(func(_ int, binding ProcessorBinding) bool {
		processor := binding.Processor
		if processor == nil || binding.Mode == ModeDisabled {
			return true
		}
		result, err := processor.Process(runCtx, input)
		result = normalizeResult(result, processor.Name(), binding.Mode)
		if err != nil {
			if result.Status == StatusSucceeded {
				result.Status = StatusFailed
			}
			result.Error = err.Error()
			runErr = errors.Join(runErr, err)
		}
		if result.Status == StatusSkipped && isStrictBinding(binding) {
			if strings.TrimSpace(result.Error) == "" {
				result.Error = "strict processor skipped"
			}
			runErr = errors.Join(runErr, fmt.Errorf("%w: %s", ErrProcessingFailed, result.Error))
		}
		if result.Status == StatusFailed {
			runErr = errors.Join(runErr, fmt.Errorf("%w: %s", ErrProcessingFailed, result.Error))
		}
		if result.Status == StatusBlocked {
			runErr = errors.Join(runErr, ErrProcessingDenied)
		}
		results.Add(result)
		return continueAfterProcessorError || runErr == nil
	})
	finalResults := mergeProcessorResults(base, results)
	if runErr != nil {
		if err := s.storeRecord(runCtx, input.Object, statusFromError(runErr), runErr.Error(), finalResults); err != nil && hasInlineStrictBinding(bindings) {
			runErr = errors.Join(runErr, fmt.Errorf("store processing record: %w", err))
		}
		return s.strictError(runErr)
	}
	if err := s.storeRecord(runCtx, input.Object, statusFromResults(finalResults), "", finalResults); err != nil {
		return s.bindingStoreRecordError(err, bindings)
	}
	return nil
}

func (s *Service) storeRecord(ctx context.Context, object ObjectRef, status, errorText string, results *collectionlist.List[ProcessorResult]) error {
	if s == nil {
		return nil
	}
	ctx = contextOrBackground(ctx)
	key := objectKey(object)
	if s.isDiscarded(object) {
		return nil
	}
	record := Record{
		Object:    object,
		Mode:      s.cfg.Mode,
		Status:    status,
		Error:     errorText,
		Results:   results,
		UpdatedAt: time.Now().UTC(),
	}
	var storeErr error
	if s.store != nil {
		if _, err := s.store.UpsertProcessingRecord(ctx, modelFromRecord(record)); err != nil {
			storeErr = err
			if s.logger != nil {
				s.logger.WarnContext(ctx, "upsert processing record", "bucket", object.Bucket, "key", object.Key, "version_id", object.VersionID, "error", err)
			}
		}
	}
	s.mu.Lock()
	if _, discarded := s.discarded[key]; discarded {
		s.mu.Unlock()
		s.deleteRecordFromStore(ctx, object)
		return storeErr
	}
	s.records[key] = record
	s.mu.Unlock()
	return storeErr
}

func (s *Service) discardedTombstoneCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.discardedOrder)
}
func (s *Service) rememberDiscardedLocked(key string) {
	if _, exists := s.discarded[key]; exists {
		return
	}
	s.discarded[key] = struct{}{}
	s.discardedOrder = append(s.discardedOrder, key)
	if len(s.discardedOrder) <= maxDiscardedTombstones {
		return
	}
	oldest := s.discardedOrder[0]
	s.discardedOrder = s.discardedOrder[1:]
	delete(s.discarded, oldest)
}
func (s *Service) isDiscarded(object ObjectRef) bool {
	if object.VersionID == "" {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, discarded := s.discarded[objectKey(object)]
	return discarded
}
func (s *Service) bindingStoreRecordError(err error, bindings *collectionlist.List[ProcessorBinding]) error {
	if err == nil || s == nil || !hasInlineStrictBinding(bindings) {
		return nil
	}
	return s.strictError(fmt.Errorf("store processing record: %w", err))
}

func (s *Service) strictError(err error) error {
	if s == nil || s.cfg.FailOpen {
		return nil
	}
	return err
}

func (s *Service) bindingsForModes(modes ...string) *collectionlist.List[ProcessorBinding] {
	result := collectionlist.NewList[ProcessorBinding]()
	if s == nil || s.processors == nil {
		return result
	}
	allowed := make(map[string]struct{}, len(modes))
	for _, mode := range modes {
		allowed[NormalizeMode(mode)] = struct{}{}
	}
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
		if strings.TrimSpace(errorText) == "" {
			errorText = "strict processor skipped"
		}
		return fmt.Errorf("%w: %s", ErrProcessingFailed, errorText)
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
	switch result.Status {
	case StatusSucceeded, StatusSkipped, StatusFailed, StatusBlocked:
	default:
		if strings.TrimSpace(result.Error) == "" {
			result.Error = "unknown processor status: " + result.Status
		}
		result.Status = StatusFailed
	}
	if result.CompletedAt.IsZero() {
		result.CompletedAt = time.Now().UTC()
	}
	return result
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
	status := ""
	results.Range(func(_ int, result ProcessorResult) bool {
		switch result.Status {
		case StatusBlocked:
			status = StatusBlocked
			return false
		case StatusFailed:
			status = StatusFailed
			return false
		case StatusRunning:
			status = StatusRunning
		case StatusQueued:
			if status != StatusRunning {
				status = StatusQueued
			}
		case StatusSucceeded:
			if status == "" || status == StatusSkipped {
				status = StatusSucceeded
			}
		case StatusSkipped:
			if status == "" {
				status = StatusSkipped
			}
		}
		return true
	})
	if status == "" {
		return StatusSkipped
	}
	return status
}

func objectKey(object ObjectRef) string {
	version := object.VersionID
	if version == "" {
		version = object.Digest
	}
	return object.Bucket + "\x00" + object.Key + "\x00" + version
}

func modelFromRecord(record Record) model.ProcessingRecord {
	return model.ProcessingRecord{
		Bucket:    record.Object.Bucket,
		Key:       record.Object.Key,
		VersionID: record.Object.VersionID,
		Digest:    record.Object.Digest,
		Mode:      record.Mode,
		Status:    record.Status,
		Error:     record.Error,
		Results:   marshalResults(record.Results),
		UpdatedAt: record.UpdatedAt,
	}
}

func recordFromModel(record model.ProcessingRecord) Record {
	return Record{
		Object: ObjectRef{
			Bucket:    record.Bucket,
			Key:       record.Key,
			VersionID: record.VersionID,
			Digest:    record.Digest,
		},
		Mode:      record.Mode,
		Status:    record.Status,
		Error:     record.Error,
		Results:   unmarshalResults(record.Results),
		UpdatedAt: record.UpdatedAt,
	}
}

func marshalResults(results *collectionlist.List[ProcessorResult]) string {
	if results == nil {
		return "[]"
	}
	data, err := json.Marshal(results.Values())
	if err != nil {
		return "[]"
	}
	return string(data)
}

func unmarshalResults(raw string) *collectionlist.List[ProcessorResult] {
	items := []ProcessorResult{}
	if strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &items)
	}
	return collectionlist.NewList(items...)
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
	replacements := map[string]struct{}{}
	if additions != nil {
		additions.Range(func(_ int, result ProcessorResult) bool {
			replacements[processorResultKey(result)] = struct{}{}
			return true
		})
	}
	if base != nil {
		base.Range(func(_ int, result ProcessorResult) bool {
			if _, replace := replacements[processorResultKey(result)]; !replace {
				merged.Add(result)
			}
			return true
		})
	}
	if additions != nil {
		additions.Range(func(_ int, result ProcessorResult) bool {
			merged.Add(result)
			return true
		})
	}
	return merged
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
	return ProcessorResult{
		Processor:   "noop",
		Status:      StatusSucceeded,
		CompletedAt: time.Now().UTC(),
	}, nil
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
