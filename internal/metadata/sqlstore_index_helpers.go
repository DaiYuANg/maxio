package metadata

import (
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func prepareIndexDocument(document model.IndexDocument) (model.IndexDocument, error) {
	document.ID = strings.TrimSpace(document.ID)
	document.Bucket = strings.TrimSpace(document.Bucket)
	document.Key = strings.TrimSpace(document.Key)
	document.VersionID = strings.TrimSpace(document.VersionID)
	document.Digest = strings.TrimSpace(document.Digest)
	document.State = strings.TrimSpace(document.State)
	if document.ID == "" {
		document.ID = indexEntityID(document.Bucket, document.Key, document.VersionID)
	}
	if document.ID == "" || document.Bucket == "" || document.Key == "" || document.VersionID == "" {
		return model.IndexDocument{}, ErrBadRequest
	}
	if document.State == "" {
		document.State = model.IndexDocumentStatePending
	}
	now := time.Now().UTC()
	if document.CreatedAt.IsZero() {
		document.CreatedAt = now
	}
	document.UpdatedAt = now
	return document, nil
}

func prepareIndexJob(job model.IndexJob) (model.IndexJob, error) {
	job.ID = strings.TrimSpace(job.ID)
	job.Kind = strings.TrimSpace(job.Kind)
	job.Bucket = strings.TrimSpace(job.Bucket)
	job.Key = strings.TrimSpace(job.Key)
	job.VersionID = strings.TrimSpace(job.VersionID)
	job.Status = normalizeIndexQueueStatus(job.Status, model.IndexJobStatusQueued)
	if invalidIndexQueueIdentity(job.ID, job.Kind) {
		return model.IndexJob{}, ErrBadRequest
	}
	job.AvailableAt, job.CreatedAt, job.UpdatedAt = indexQueueTimes(job.AvailableAt, job.CreatedAt)
	return job, nil
}

func prepareIndexOutboxEvent(event model.IndexOutboxEvent) (model.IndexOutboxEvent, error) {
	event.ID = strings.TrimSpace(event.ID)
	event.EventType = strings.TrimSpace(event.EventType)
	event.Bucket = strings.TrimSpace(event.Bucket)
	event.Key = strings.TrimSpace(event.Key)
	event.VersionID = strings.TrimSpace(event.VersionID)
	event.Status = normalizeIndexQueueStatus(event.Status, model.IndexOutboxStatusPending)
	if invalidIndexQueueIdentity(event.ID, event.EventType) {
		return model.IndexOutboxEvent{}, ErrBadRequest
	}
	event.AvailableAt, event.CreatedAt, event.UpdatedAt = indexQueueTimes(event.AvailableAt, event.CreatedAt)
	return event, nil
}

func normalizeIndexQueueStatus(status, fallback string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return fallback
	}
	return status
}

func invalidIndexQueueIdentity(id, kind string) bool {
	return id == "" || kind == ""
}

func indexQueueTimes(availableAt, createdAt time.Time) (time.Time, time.Time, time.Time) {
	now := time.Now().UTC()
	if availableAt.IsZero() {
		availableAt = now
	}
	if createdAt.IsZero() {
		createdAt = now
	}
	return availableAt, createdAt, now
}
