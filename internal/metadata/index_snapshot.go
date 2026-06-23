package metadata

import (
	"context"
	"errors"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var errIndexSnapshotRepositoryUnavailable = errors.New("metadata repository unavailable")

// IndexSnapshot is a metadata-first view of derived index state.
type IndexSnapshot struct {
	PendingDocuments       int
	IndexedDocuments       int
	DeletedDocuments       int
	FailedDocuments        int
	QueuedJobs             int
	RunningJobs            int
	SucceededJobs          int
	FailedJobs             int
	RetriedJobs            int
	PendingOutboxEvents    int
	DispatchedOutboxEvents int
	FailedOutboxEvents     int
	RebuildRunning         bool
	LastIndexedAt          time.Time
	LastError              string
	LastRebuildStartedAt   time.Time
	LastRebuildFinishedAt  time.Time
	LastRebuildError       string

	lastErrorAt   time.Time
	lastRebuildAt time.Time
}

// CollectIndexSnapshot aggregates index document, job, and outbox metadata
// without going through the legacy object service.
func CollectIndexSnapshot(ctx context.Context, store Repository, limit int) (IndexSnapshot, error) {
	if store == nil {
		return IndexSnapshot{}, errIndexSnapshotRepositoryUnavailable
	}

	snapshot := IndexSnapshot{}
	documents, err := store.ListIndexDocuments(ctx, "", "")
	if err != nil {
		return snapshot, err
	}
	snapshot.addIndexDocuments(documents)

	limit = normalizeListLimit(limit)
	jobs, err := store.ListIndexJobs(ctx, "", limit)
	if err != nil {
		return snapshot, err
	}
	snapshot.addIndexJobs(jobs)

	events, err := store.ListIndexOutboxEvents(ctx, "", limit)
	if err != nil {
		return snapshot, err
	}
	snapshot.addIndexOutboxEvents(events)
	return snapshot, nil
}

func (snapshot *IndexSnapshot) addIndexDocuments(documents *collectionlist.List[model.IndexDocument]) {
	if documents == nil {
		return
	}
	documents.Range(func(_ int, document model.IndexDocument) bool {
		snapshot.addIndexDocument(document)
		return true
	})
}

func (snapshot *IndexSnapshot) addIndexDocument(document model.IndexDocument) {
	switch document.State {
	case model.IndexDocumentStateIndexed:
		snapshot.IndexedDocuments++
	case model.IndexDocumentStateDeleted:
		snapshot.DeletedDocuments++
	case model.IndexDocumentStateFailed:
		snapshot.FailedDocuments++
	default:
		snapshot.PendingDocuments++
	}
	if document.IndexedAt.After(snapshot.LastIndexedAt) {
		snapshot.LastIndexedAt = document.IndexedAt
	}
	snapshot.trackError(document.Error, document.UpdatedAt)
}

func (snapshot *IndexSnapshot) addIndexJobs(jobs *collectionlist.List[model.IndexJob]) {
	if jobs == nil {
		return
	}
	jobs.Range(func(_ int, job model.IndexJob) bool {
		snapshot.addIndexJob(job)
		return true
	})
}

func (snapshot *IndexSnapshot) addIndexJob(job model.IndexJob) {
	switch job.Status {
	case model.IndexJobStatusRunning:
		snapshot.RunningJobs++
	case model.IndexJobStatusSucceeded:
		snapshot.SucceededJobs++
	case model.IndexJobStatusFailed:
		snapshot.FailedJobs++
	default:
		snapshot.QueuedJobs++
	}
	if job.Attempts > 1 {
		snapshot.RetriedJobs++
	}
	snapshot.trackError(job.Error, job.UpdatedAt)
	if job.Kind == model.IndexJobKindRebuild {
		snapshot.addRebuildJob(job)
	}
}

func (snapshot *IndexSnapshot) addRebuildJob(job model.IndexJob) {
	if job.Status == model.IndexJobStatusRunning {
		snapshot.RebuildRunning = true
	}
	updatedAt := job.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = job.CreatedAt
	}
	if !snapshot.lastRebuildAt.IsZero() && !updatedAt.After(snapshot.lastRebuildAt) {
		return
	}
	snapshot.lastRebuildAt = updatedAt
	snapshot.LastRebuildStartedAt = job.StartedAt
	if snapshot.LastRebuildStartedAt.IsZero() {
		snapshot.LastRebuildStartedAt = job.CreatedAt
	}
	snapshot.LastRebuildFinishedAt = job.FinishedAt
	snapshot.LastRebuildError = job.Error
}

func (snapshot *IndexSnapshot) addIndexOutboxEvents(events *collectionlist.List[model.IndexOutboxEvent]) {
	if events == nil {
		return
	}
	events.Range(func(_ int, event model.IndexOutboxEvent) bool {
		snapshot.addIndexOutboxEvent(event)
		return true
	})
}

func (snapshot *IndexSnapshot) addIndexOutboxEvent(event model.IndexOutboxEvent) {
	switch event.Status {
	case model.IndexOutboxStatusDispatched:
		snapshot.DispatchedOutboxEvents++
	case model.IndexOutboxStatusFailed:
		snapshot.FailedOutboxEvents++
	default:
		snapshot.PendingOutboxEvents++
	}
	snapshot.trackError(event.Error, event.UpdatedAt)
}

func (snapshot *IndexSnapshot) trackError(message string, at time.Time) {
	if message == "" {
		return
	}
	if snapshot.LastError == "" || snapshot.lastErrorAt.IsZero() || at.After(snapshot.lastErrorAt) {
		snapshot.LastError = message
		snapshot.lastErrorAt = at
	}
}
