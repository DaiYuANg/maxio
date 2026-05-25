package object

import (
	"context"
	"fmt"
	"time"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/maxio/internal/index"
)

type IndexStatus struct {
	Rebuilding            bool      `json:"rebuilding"`
	QueueSize             int       `json:"queue_size"`
	QueuedObjects         int       `json:"queued_objects"`
	DroppedObjects        int       `json:"dropped_objects"`
	RetriedObjects        int       `json:"retried_objects"`
	IndexedObjects        int       `json:"indexed_objects"`
	FailedObjects         int       `json:"failed_objects"`
	LastIndexedAt         time.Time `json:"last_indexed_at,omitzero"`
	LastError             string    `json:"last_error,omitempty"`
	LastRebuildStartedAt  time.Time `json:"last_rebuild_started_at,omitzero"`
	LastRebuildFinishedAt time.Time `json:"last_rebuild_finished_at,omitzero"`
	LastRebuildObjects    int       `json:"last_rebuild_objects"`
	LastRebuildFailed     int       `json:"last_rebuild_failed"`
	LastRebuildError      string    `json:"last_rebuild_error,omitempty"`
}

type indexTask struct {
	Event   string
	Bucket  string
	Key     string
	Attempt int
}

const (
	defaultIndexBatchSize   = 32
)

func (s *Service) StartIndexWorker(ctx context.Context) error {
	if s == nil || s.bus == nil || s.search == nil {
		return nil
	}

	s.indexMu.Lock()
	if s.indexCh != nil {
		s.indexMu.Unlock()
		return nil
	}
	s.indexCh = make(chan indexTask, s.indexQueueSize())
	s.indexWg.Add(1)
	s.indexMu.Unlock()

	go s.runIndexWorker(context.WithoutCancel(ctx))

	_, err := eventx.Subscribe(s.bus, func(_ context.Context, event ObjectEvent) error {
		s.enqueueIndexEvent(event)
		return nil
	})
	if err != nil {
		if stopErr := s.stopIndexWorker(); stopErr != nil {
			return fmt.Errorf("subscribe object index worker: %w; stop index worker: %w", err, stopErr)
		}
		return fmt.Errorf("subscribe object index worker: %w", err)
	}
	return nil
}

func (s *Service) closeIndexChannel() {
	s.indexMu.Lock()
	if s.indexCh == nil {
		s.indexMu.Unlock()
		return
	}
	ch := s.indexCh
	s.indexCh = nil
	s.indexMu.Unlock()
	close(ch)
}

func (s *Service) stopIndexWorker() error {
	if s == nil {
		return nil
	}
	s.closeIndexChannel()
	s.indexWg.Wait()
	return nil
}

func (s *Service) enqueueIndexEvent(event ObjectEvent) {
	bucket, key := eventObjectLocation(event)
	if bucket == "" || key == "" {
		return
	}
	task := indexTask{Event: event.Event, Bucket: bucket, Key: key}
	if !s.tryEnqueueIndexTask(task) {
		s.recordIndexDrop()
		s.logger.Warn("index queue full, dropping object event", "event", event.Event, "bucket", bucket, "key", key)
	}
}

func (s *Service) tryEnqueueIndexTask(task indexTask) (sent bool) {
	ch := s.indexChannel()
	if ch == nil {
		return false
	}
	defer func() {
		if recover() != nil {
			sent = false
		}
	}()

	select {
	case ch <- task:
		return true
	default:
		return false
	}
}

func (s *Service) indexChannel() chan indexTask {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	return s.indexCh
}

func (s *Service) runIndexWorker(ctx context.Context) {
	defer s.indexWg.Done()
	ch := s.indexChannel()
	if ch == nil {
		return
	}
	for {
		first, ok := s.waitIndexTask(ctx, ch)
		if !ok {
			return
		}
		batch := []indexTask{first}
		batch = s.coalesceIndexTasks(ctx, ch, batch)
		s.handleIndexTaskBatch(ctx, batch)
	}
}

func (s *Service) waitIndexTask(ctx context.Context, ch chan indexTask) (indexTask, bool) {
	if ch == nil {
		return indexTask{}, false
	}
	select {
	case task, ok := <-ch:
		return task, ok
	case <-ctx.Done():
		return indexTask{}, false
	}
}

func (s *Service) coalesceIndexTasks(ctx context.Context, ch chan indexTask, tasks []indexTask) []indexTask {
	if ch == nil {
		return tasks
	}

	maxBatch := s.indexBatchSize()
	if maxBatch <= 1 {
		return tasks[:1]
	}

	for len(tasks) < maxBatch {
		select {
		case task, ok := <-ch:
			if !ok {
				return tasks
			}
			tasks = append(tasks, task)
		case <-ctx.Done():
			return tasks
		default:
			return tasks
		}
	}
	return tasks
}

func (s *Service) handleIndexTaskBatch(ctx context.Context, tasks []indexTask) {
	if len(tasks) == 0 {
		return
	}

	tasks = s.deduplicateIndexTasks(tasks)
	timeout := s.cfg.IndexTimeoutDuration()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	rateLimiter := s.indexRateLimiter()
	if rateLimiter != nil {
		defer rateLimiter.Stop()
	}
	docs := s.collectIndexDocs(ctx, tasks, rateLimiter)
	s.flushIndexDocs(ctx, docs)
}

func (s *Service) collectIndexDocs(
	ctx context.Context,
	tasks []indexTask,
	rateTicker *time.Ticker,
) []index.IndexDocument {
	docs := make([]index.IndexDocument, 0, len(tasks))
	for _, task := range tasks {
		if rateTicker != nil && !waitForNextBatchTick(ctx, rateTicker) {
			return docs
		}
		docs = s.appendIndexDoc(ctx, task, docs)
	}
	return docs
}

func (s *Service) appendIndexDoc(
	ctx context.Context,
	task indexTask,
	docs []index.IndexDocument,
) []index.IndexDocument {
	switch task.Event {
	case "object.updated":
		doc, hadTextError, err := s.buildIndexDocument(ctx, task.Bucket, task.Key)
		if err != nil {
			if s.retryIndexTask(task, err) {
				return docs
			}
			if hadTextError && doc.Meta.Bucket != "" && doc.Meta.Key != "" {
				s.search.UpsertDocument(doc.Meta, doc.Text)
			}
			s.recordIndexFailure(err, false)
			s.logger.WarnContext(ctx, "index object failed", "bucket", task.Bucket, "key", task.Key, "error", err)
			return docs
		}
		return append(docs, doc)
	case "object.deleted":
		s.search.Remove(task.Bucket, task.Key)
	}
	return docs
}

func (s *Service) flushIndexDocs(ctx context.Context, docs []index.IndexDocument) {
	if len(docs) == 0 {
		return
	}

	successes, err := s.search.UpsertDocuments(docs)
	s.recordIndexSuccessCount(successes)
	if err == nil {
		return
	}

	errorCount := len(docs) - successes
	if errorCount > 0 {
		s.recordIndexFailureCount(errorCount, err)
	}
	s.logger.WarnContext(ctx, "batch index upsert failed", "count", len(docs), "error", err)
}

func (s *Service) buildIndexDocument(
	ctx context.Context,
	bucket,
	key string,
) (index.IndexDocument, bool, error) {
	body, meta, err := s.GetObject(ctx, bucket, key)
	if err != nil {
		return index.IndexDocument{}, false, fmt.Errorf("load object for indexing: %w", err)
	}
	defer closeIndexBody(ctx, s, body)

	text, err := index.ExtractText(body, meta)
	if err != nil {
		return index.IndexDocument{
			Meta: meta,
			Text: "",
		}, true, fmt.Errorf("extract object text: %w", err)
	}
	return index.IndexDocument{
		Meta: meta,
		Text: text,
	}, false, nil
}

func (s *Service) indexObject(ctx context.Context, bucket, key string) error {
	doc, hadTextError, err := s.buildIndexDocument(ctx, bucket, key)
	if err != nil {
		if hadTextError && doc.Meta.Bucket != "" && doc.Meta.Key != "" {
			s.search.UpsertDocument(doc.Meta, doc.Text)
		}
		return err
	}
	s.search.UpsertDocument(doc.Meta, doc.Text)
	return nil
}
