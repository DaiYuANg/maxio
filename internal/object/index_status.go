package object

import "time"

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

type IndexRebuildResult struct {
	Objects    int       `json:"objects"`
	Failed     int       `json:"failed"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
}
