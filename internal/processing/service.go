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
		s.storeRecordOrWarn(ctx, input.Object, record.Status, record.Error, base, "promote digest processing record")
		s.discardDigestRecord(ctx, input.Object)
	}

	asyncBindings := s.bindingsForModes(ModeAsyncPermissive, ModeAsyncStrict)
	if asyncBindings.Len() > 0 {
		queued := mergeProcessorResults(base, processorStatusResults(asyncBindings, StatusQueued))
		s.storeRecordOrWarn(ctx, input.Object, statusFromResults(queued), "", queued, "queue async processing record")
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
	s.storeRecordOrWarn(ctx, input.Object, StatusSucceeded, "", collectionlist.NewList[ProcessorResult](), "store completed processing record")
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
