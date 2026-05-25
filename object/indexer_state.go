package object

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

func (s *Service) retryIndexTask(task indexTask, cause error) bool {
	if task.Attempt >= s.cfg.IndexMaxRetries {
		return false
	}
	task.Attempt++
	backoff := s.cfg.IndexRetryBackoffDuration()
	if backoff <= 0 {
		backoff = time.Second
	}
	timer := time.NewTimer(backoff)
	go func() {
		defer timer.Stop()
		<-timer.C
		if s.tryEnqueueIndexTask(task) {
			s.recordIndexRetry()
		} else {
			s.recordIndexFailure(cause, true)
		}
	}()
	return true
}

func (s *Service) recordIndexSuccess() {
	s.recordIndexResult(1, 0, false, nil)
}

func (s *Service) recordIndexSuccessCount(count int) {
	if count <= 0 {
		return
	}
	s.recordIndexResult(count, 0, false, nil)
}

func (s *Service) recordIndexFailure(err error, dropped bool) {
	s.recordIndexResult(0, 1, dropped, err)
}

func (s *Service) recordIndexFailureCount(count int, err error) {
	if count <= 0 {
		return
	}
	s.recordIndexResult(0, count, false, err)
}

func (s *Service) recordIndexRetry() {
	s.recordIndexResult(0, 0, false, nil)
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	s.index.RetriedObjects++
}

func (s *Service) recordIndexDrop() {
	s.recordIndexResult(0, 0, true, nil)
}

func (s *Service) recordIndexResult(indexed, failed int, dropped bool, err error) {
	s.indexMu.Lock()
	defer s.indexMu.Unlock()
	s.index.IndexedObjects += indexed
	s.index.FailedObjects += failed
	if dropped {
		s.index.DroppedObjects++
	}
	s.refreshIndexQueueStatusLocked()
	s.index.LastIndexedAt = time.Now().UTC()
	if err != nil {
		s.index.LastError = err.Error()
	}
}

func (s *Service) refreshIndexQueueStatusLocked() {
	s.index.QueuedObjects = len(s.indexCh)
	s.index.QueueSize = cap(s.indexCh)
}

func (s *Service) indexQueueSize() int {
	if s.cfg.IndexQueueSize <= 0 {
		return 1024
	}
	return s.cfg.IndexQueueSize
}

func (s *Service) indexBatchSize() int {
	queueSize := s.indexQueueSize()
	if queueSize <= defaultIndexBatchSize {
		return queueSize
	}
	if queueSize > 4*defaultIndexBatchSize {
		return defaultIndexBatchSize
	}
	return queueSize
}

func (s *Service) indexRateLimiter() *time.Ticker {
	if s.cfg.IndexRateLimit <= 0 {
		return nil
	}
	return time.NewTicker(time.Second / time.Duration(s.cfg.IndexRateLimit))
}

func eventObjectLocation(event ObjectEvent) (string, string) {
	bucket := strings.TrimSpace(payloadString(event.Payload, "bucket"))
	key := strings.TrimSpace(payloadString(event.Payload, "key"))
	return bucket, key
}

func payloadString(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func closeIndexBody(_ context.Context, s *Service, body io.Closer) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil {
		s.logger.Warn("close object indexing body failed", "error", err)
	}
}
