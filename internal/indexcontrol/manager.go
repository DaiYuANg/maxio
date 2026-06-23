// Package indexcontrol provides metadata-backed control operations for the
// search index HTTP endpoints.
package indexcontrol

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	searchindex "github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const defaultManagerStatusLimit = 1000

var (
	ErrManagerUnavailable = errors.New("index manager unavailable")
	ErrRebuildInProgress  = errors.New("index rebuild already in progress")
)

// Status is the handler-facing index status shape. It intentionally keeps the
// legacy JSON field names so /_index/status can move off object.Service without
// changing clients.
type Status struct {
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

// RebuildResult is the handler-facing result shape for /_index/rebuild.
type RebuildResult struct {
	Objects    int       `json:"objects"`
	Failed     int       `json:"failed"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}

type ManagerOptions struct {
	StatusLimit int
	Now         func() time.Time
}

type Manager struct {
	metadata metadata.Repository
	search   *searchindex.SearchEngine
	options  ManagerOptions
	mu       sync.RWMutex
	rebuild  rebuildState
}

type rebuildState struct {
	running   bool
	last      RebuildResult
	lastError string
}

func NewManager(store metadata.Repository, search *searchindex.SearchEngine, options ManagerOptions) *Manager {
	return &Manager{
		metadata: store,
		search:   search,
		options:  options.normalized(),
	}
}

func (manager *Manager) Status(ctx context.Context) (Status, error) {
	if manager == nil || manager.metadata == nil {
		return Status{}, ErrManagerUnavailable
	}
	ctx = ensureManagerContext(ctx)
	options := manager.options.normalized()
	snapshot, err := metadata.CollectIndexSnapshot(ctx, manager.metadata, options.StatusLimit)
	if err != nil {
		return Status{}, fmt.Errorf("collect index status: %w", err)
	}
	status := statusFromSnapshot(snapshot)
	status.applyRebuildState(manager.rebuildSnapshot())
	return status, nil
}

func (manager *Manager) Rebuild(ctx context.Context) (result RebuildResult, err error) {
	if manager == nil || manager.metadata == nil || manager.search == nil {
		return RebuildResult{}, ErrManagerUnavailable
	}
	ctx = ensureManagerContext(ctx)
	options := manager.options.normalized()
	result.StartedAt = options.now()
	if !manager.beginRebuild(result.StartedAt) {
		return RebuildResult{}, ErrRebuildInProgress
	}

	jobID := rebuildJobID(result.StartedAt)
	rebuildError := manager.startRebuildJob(ctx, jobID, result)
	defer func() {
		result, err = manager.completeRebuild(ctx, jobID, result, err, rebuildError, options.now)
	}()

	objects, err := manager.listCommittedObjectMetas(ctx)
	if err != nil {
		return result, fmt.Errorf("list committed object metadata: %w", err)
	}
	indexed, failed, message := manager.rebuildObjects(ctx, objects, options.now)
	result.Objects = indexed
	result.Failed = failed
	rebuildError = firstMessage(rebuildError, message)
	rebuildError = firstErrorMessage(rebuildError, manager.pruneObjects(objects, &result))
	return result, nil
}

func (manager *Manager) startRebuildJob(ctx context.Context, jobID string, result RebuildResult) string {
	if err := manager.markRebuildJob(ctx, jobID, model.IndexJobStatusRunning, result, ""); err != nil {
		return failureMessage(err)
	}
	return ""
}

func (manager *Manager) completeRebuild(
	ctx context.Context,
	jobID string,
	result RebuildResult,
	rebuildErr error,
	rebuildMessage string,
	now func() time.Time,
) (RebuildResult, error) {
	result.FinishedAt = now()
	rebuildMessage = firstErrorMessage(rebuildMessage, rebuildErr)
	manager.finishRebuild(result, rebuildMessage)
	status := rebuildJobStatus(result, rebuildErr, rebuildMessage)
	if markErr := manager.markRebuildJob(ctx, jobID, status, result, rebuildMessage); markErr != nil && rebuildErr == nil {
		return result, markErr
	}
	return result, rebuildErr
}

func (manager *Manager) beginRebuild(startedAt time.Time) bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.rebuild.running {
		return false
	}
	manager.rebuild.running = true
	manager.rebuild.last = RebuildResult{StartedAt: startedAt}
	manager.rebuild.lastError = ""
	return true
}

func (manager *Manager) finishRebuild(result RebuildResult, message string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.rebuild.running = false
	manager.rebuild.last = result
	manager.rebuild.lastError = message
}

func (manager *Manager) rebuildSnapshot() rebuildState {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.rebuild
}

func ensureManagerContext(ctx context.Context) context.Context {
	if ctx != nil {
		return ctx
	}
	return context.Background()
}

func (options ManagerOptions) normalized() ManagerOptions {
	if options.StatusLimit <= 0 {
		options.StatusLimit = defaultManagerStatusLimit
	}
	if options.Now == nil {
		options.Now = func() time.Time {
			return time.Now().UTC()
		}
	}
	return options
}

func (options ManagerOptions) now() time.Time {
	return options.Now().UTC()
}
