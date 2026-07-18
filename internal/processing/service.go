package processing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
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
	inlineBindings *collectionlist.List[ProcessorBinding]
	asyncBindings  *collectionlist.List[ProcessorBinding]
	strictBindings *collectionlist.List[ProcessorBinding]
	mu             sync.RWMutex
	records        map[string]Record
	discarded      map[string]struct{}
	discardedOrder []string
	backgroundMu   sync.Mutex
	backgroundDone bool
	backgroundNext uint64
	backgroundStop map[uint64]context.CancelFunc
	backgroundWG   sync.WaitGroup
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
	bindings := normalizeProcessorBindings(cfg.Mode, processors...)
	return &Service{
		logger:         logger,
		cfg:            cfg,
		store:          store,
		processors:     bindings,
		inlineBindings: filterProcessorBindings(bindings, ModeInlineStrict),
		asyncBindings:  filterProcessorBindings(bindings, ModeAsyncPermissive, ModeAsyncStrict),
		strictBindings: filterProcessorBindings(bindings, ModeInlineStrict, ModeAsyncStrict),
		records:        make(map[string]Record),
		discarded:      make(map[string]struct{}),
		discardedOrder: make([]string, 0),
		backgroundStop: make(map[uint64]context.CancelFunc),
	}
}

func (s *Service) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	ctx = contextOrBackground(ctx)
	s.cancelBackgroundTasks()
	done := make(chan struct{})
	go func() {
		s.backgroundWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("close processing service: %w", ctx.Err())
	}
}

func (s *Service) cancelBackgroundTasks() {
	s.backgroundMu.Lock()
	defer s.backgroundMu.Unlock()
	if s.backgroundDone {
		return
	}
	s.backgroundDone = true
	for id, cancel := range s.backgroundStop {
		cancel()
		delete(s.backgroundStop, id)
	}
}

func (s *Service) beginBackgroundTask(parent context.Context) (context.Context, context.CancelFunc, bool) {
	if s == nil {
		return nil, nil, false
	}
	ctx, cancel := context.WithTimeout(detachedTaskParent(parent), s.cfg.Timeout)
	s.backgroundMu.Lock()
	defer s.backgroundMu.Unlock()
	if s.backgroundDone {
		cancel()
		return nil, nil, false
	}
	s.backgroundNext++
	id := s.backgroundNext
	s.backgroundStop[id] = cancel
	s.backgroundWG.Add(1)
	return ctx, func() {
		cancel()
		s.finishBackgroundTask(id)
	}, true
}

func (s *Service) finishBackgroundTask(id uint64) {
	s.backgroundMu.Lock()
	delete(s.backgroundStop, id)
	s.backgroundMu.Unlock()
	s.backgroundWG.Done()
}

type detachedTaskContext struct {
	parent context.Context
}

func detachedTaskParent(parent context.Context) context.Context {
	if parent == nil {
		return context.Background()
	}
	return detachedTaskContext{parent: parent}
}

func (ctx detachedTaskContext) Deadline() (time.Time, bool) {
	return time.Time{}, false
}

func (ctx detachedTaskContext) Done() <-chan struct{} {
	return nil
}

func (ctx detachedTaskContext) Err() error {
	return nil
}

func (ctx detachedTaskContext) Value(key any) any {
	return ctx.parent.Value(key)
}
func (s *Service) ProcessBeforeCommit(ctx context.Context, input Input) error {
	if s == nil || !s.cfg.Enabled {
		return nil
	}
	bindings := s.inlineStrictBindings()
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
		s.storeRecordOrWarn(ctx, input.Object, record.Status, record.Error, base, "promote digest processing record")
		s.discardDigestRecord(ctx, input.Object)
	}

	asyncBindings := s.asyncProcessorBindings()
	if asyncBindings.Len() > 0 {
		queued := mergeProcessorResults(base, processorStatusResults(asyncBindings, StatusQueued))
		s.storeRecordOrWarn(ctx, input.Object, statusFromResults(queued), "", queued, "queue async processing record")
		s.startAsyncProcessing(ctx, input, asyncBindings, cloneProcessorResults(base))
		return
	}

	if recordExists {
		return
	}
	s.storeRecordOrWarn(ctx, input.Object, StatusSucceeded, "", collectionlist.NewList[ProcessorResult](), "store completed processing record")
}

func (s *Service) startAsyncProcessing(
	ctx context.Context,
	input Input,
	asyncBindings *collectionlist.List[ProcessorBinding],
	base *collectionlist.List[ProcessorResult],
) {
	logCtx := contextOrBackground(ctx)
	runCtx, finish, ok := s.beginBackgroundTask(ctx)
	if !ok {
		if s.logger != nil {
			s.logger.WarnContext(logCtx, "skip object post-commit processing: service is closing", "bucket", input.Object.Bucket, "key", input.Object.Key, "version_id", input.Object.VersionID)
		}
		return
	}
	go func() {
		defer finish()
		defer cleanupInput(runCtx, input)
		if err := s.run(runCtx, input, asyncBindings, base); err != nil && s.logger != nil {
			s.logger.WarnContext(runCtx, "object post-commit processing failed", "bucket", input.Object.Bucket, "key", input.Object.Key, "version_id", input.Object.VersionID, "error", err)
		}
	}()
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
