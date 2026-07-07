package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/lyonbrown4d/maxio/internal/processing"
)

func (m *dedupeMiddleware) processPutBeforeCommit(
	ctx context.Context,
	w http.ResponseWriter,
	event ObjectPutSucceededEvent,
	captured *capturedRequestBody,
) bool {
	if m == nil || m.processor == nil {
		return true
	}
	input := processingInputFromPutEvent(event, captured)
	if err := m.processor.ProcessBeforeCommit(ctx, input); err != nil {
		writeS3ProxyProcessingError(w, err)
		m.logger.WarnContext(ctx, "s3 put rejected by object processing", "bucket", event.Bucket, "key", event.Key, "version_id", event.VersionID, "error", err)
		return false
	}
	return true
}

func (m *dedupeMiddleware) processPutAfterCommit(ctx context.Context, event ObjectPutSucceededEvent, captured *capturedRequestBody) {
	if m == nil || m.processor == nil {
		return
	}
	input := processing.Input{Object: processingObjectRefFromPutEvent(event)}
	snapshot := m.processor.Snapshot()
	if processingSnapshotNeedsAsyncBody(snapshot) && captured != nil {
		retained, err := captured.Clone()
		if err != nil {
			m.logger.WarnContext(ctx, "retain s3 put body for object processing", "bucket", event.Bucket, "key", event.Key, "version_id", event.VersionID, "error", err)
		} else {
			input = processingInputFromPutEvent(event, retained)
			input.Cleanup = func(cleanupCtx context.Context) {
				cleanupCapturedBody(cleanupCtx, m.logger, retained)
			}
		}
	}
	m.processor.ProcessAfterCommit(ctx, input)
}

func (m *dedupeMiddleware) ensureProcessingReadAllowed(ctx context.Context, w http.ResponseWriter, version model.ObjectVersion) bool {
	if m == nil || m.processor == nil {
		return true
	}
	if err := m.processor.EnsureReadAllowed(ctx, processingObjectRefFromVersion(version)); err != nil {
		writeS3ProxyProcessingError(w, err)
		m.logger.WarnContext(ctx, "s3 read blocked by object processing", "bucket", version.Bucket, "key", version.Key, "version_id", version.VersionID, "error", err)
		return false
	}
	return true
}

func (m *dedupeMiddleware) discardProcessingRecord(ctx context.Context, version model.ObjectVersion) {
	if m == nil || m.processor == nil {
		return
	}
	m.processor.Discard(ctx, processingObjectRefFromVersion(version))
}

func (m *dedupeMiddleware) discardPutProcessingRecord(ctx context.Context, event ObjectPutSucceededEvent) {
	if m == nil || m.processor == nil {
		return
	}
	m.processor.Discard(ctx, processingObjectRefFromPutEvent(event))
}

func processingSnapshotNeedsAsyncBody(snapshot processing.Snapshot) bool {
	if !snapshot.Enabled || snapshot.Mode == processing.ModeDisabled {
		return false
	}
	if len(snapshot.ProcessorModes) > 0 {
		for _, mode := range snapshot.ProcessorModes {
			switch processing.NormalizeMode(mode) {
			case processing.ModeAsyncPermissive, processing.ModeAsyncStrict:
				return true
			}
		}
		return false
	}
	switch processing.NormalizeMode(snapshot.Mode) {
	case processing.ModeAsyncPermissive, processing.ModeAsyncStrict:
		return true
	default:
		return false
	}
}
func processingInputFromPutEvent(event ObjectPutSucceededEvent, captured *capturedRequestBody) processing.Input {
	input := processing.Input{Object: processingObjectRefFromPutEvent(event)}
	if captured != nil {
		input.OpenContent = func(context.Context) (io.ReadCloser, error) {
			return captured.Open()
		}
	}
	return input
}

func processingObjectRefFromPutEvent(event ObjectPutSucceededEvent) processing.ObjectRef {
	return processing.ObjectRef{
		Bucket:         event.Bucket,
		Key:            event.Key,
		VersionID:      event.VersionID,
		Digest:         event.Digest,
		ETag:           event.ETag,
		Size:           event.Size,
		ContentType:    event.ContentType,
		UpstreamID:     event.UpstreamID,
		UpstreamBucket: event.UpstreamBucket,
		UpstreamKey:    event.UpstreamKey,
		UserMetadata:   event.UserMetadata,
	}
}

func processingObjectRefFromVersion(version model.ObjectVersion) processing.ObjectRef {
	return processing.ObjectRef{
		Bucket:         version.Bucket,
		Key:            version.Key,
		VersionID:      version.VersionID,
		Digest:         version.Digest,
		ETag:           version.ETag,
		Size:           version.Size,
		ContentType:    version.ContentType,
		UpstreamID:     version.UpstreamID,
		UpstreamBucket: version.UpstreamBucket,
		UpstreamKey:    version.UpstreamKey,
		UserMetadata:   version.UserMetadata,
	}
}

func writeS3ProxyProcessingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, processing.ErrProcessingPending):
		writeS3ProxyError(w, http.StatusLocked, "ObjectProcessingPending", "object processing has not completed")
	case errors.Is(err, processing.ErrProcessingDenied):
		writeS3ProxyError(w, http.StatusForbidden, "ObjectProcessingDenied", "object processing denied this object")
	case errors.Is(err, processing.ErrProcessingFailed):
		writeS3ProxyError(w, http.StatusForbidden, "ObjectProcessingFailed", "object processing failed")
	default:
		writeS3ProxyInternalError(w, "object processing failed")
	}
}
