package index

import "github.com/lyonbrown4d/maxio/internal/metadata"

func statusFromSnapshot(snapshot metadata.IndexSnapshot) Status {
	return Status{
		Rebuilding:            snapshot.RebuildRunning,
		QueueSize:             snapshot.QueuedJobs,
		QueuedObjects:         snapshot.PendingDocuments + snapshot.QueuedJobs + snapshot.PendingOutboxEvents,
		RetriedObjects:        snapshot.RetriedJobs,
		IndexedObjects:        snapshot.IndexedDocuments,
		FailedObjects:         snapshot.FailedDocuments + snapshot.FailedJobs + snapshot.FailedOutboxEvents,
		LastIndexedAt:         snapshot.LastIndexedAt,
		LastError:             snapshot.LastError,
		LastRebuildStartedAt:  snapshot.LastRebuildStartedAt,
		LastRebuildFinishedAt: snapshot.LastRebuildFinishedAt,
		LastRebuildError:      snapshot.LastRebuildError,
	}
}

func (status *Status) applyRebuildState(state rebuildState) {
	if state.running {
		status.Rebuilding = true
	}
	if state.last.StartedAt.IsZero() {
		return
	}
	status.LastRebuildStartedAt = state.last.StartedAt
	status.LastRebuildFinishedAt = state.last.FinishedAt
	status.LastRebuildObjects = state.last.Objects
	status.LastRebuildFailed = state.last.Failed
	status.LastRebuildError = state.lastError
}
