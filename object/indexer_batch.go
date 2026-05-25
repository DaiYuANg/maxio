package object

import (
	"context"
	"time"
)

func (s *Service) deduplicateIndexTasks(tasks []indexTask) []indexTask {
	if len(tasks) <= 1 {
		return tasks
	}
	type taskKey struct {
		bucket string
		key    string
	}
	latest := make(map[taskKey]indexTask, len(tasks))
	order := make([]taskKey, 0, len(tasks))
	for _, task := range tasks {
		key := taskKey{
			bucket: task.Bucket,
			key:    task.Key,
		}
		if _, exists := latest[key]; !exists {
			order = append(order, key)
		}
		latest[key] = task
	}
	deduped := make([]indexTask, 0, len(order))
	for _, key := range order {
		task := latest[key]
		deduped = append(deduped, task)
	}
	return deduped
}

func waitForNextBatchTick(ctx context.Context, ticker *time.Ticker) bool {
	if ticker == nil {
		return true
	}
	if ctx == nil {
		<-ticker.C
		return true
	}
	select {
	case <-ctx.Done():
		return false
	case <-ticker.C:
		return true
	}
}
