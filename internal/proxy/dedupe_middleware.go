package proxy

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/arcgolabs/eventx"
	"github.com/arcgolabs/vale"
	valeruntime "github.com/arcgolabs/vale/runtime"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const (
	dedupeMiddlewareType       = "maxio-dedupe"
	dedupeMiddlewareNamePrefix = "maxio-dedupe:"
)

const (
	eventS3ProxyObjectPutSucceeded    = "s3.proxy.object.put.succeeded"
	eventS3ProxyObjectDeleteSucceeded = "s3.proxy.object.delete.succeeded"
)

type ObjectPutSucceededEvent struct {
	Bucket         string
	Key            string
	Digest         string
	Size           int64
	ETag           string
	ContentType    string
	UpstreamID     string
	UpstreamBucket string
	UpstreamKey    string
	VersionID      string
	UserMetadata   map[string]string
	DedupeHit      bool
}

func (event ObjectPutSucceededEvent) Name() string {
	return eventS3ProxyObjectPutSucceeded
}

type ObjectDeleteSucceededEvent struct {
	Bucket  string
	Key     string
	Digest  string
	Deleted bool
}

func (event ObjectDeleteSucceededEvent) Name() string {
	return eventS3ProxyObjectDeleteSucceeded
}

type dedupeMiddleware struct {
	bus    eventx.BusRuntime
	store  metadata.MetadataStore
	logger *slog.Logger
}

func NewDedupeMiddlewareRegistry(
	bus eventx.BusRuntime,
	store metadata.MetadataStore,
	logger *slog.Logger,
) *vale.MiddlewareRegistry {
	if logger == nil {
		logger = slog.Default()
	}
	registry := vale.DefaultMiddlewareRegistry()
	middleware := &dedupeMiddleware{bus: bus, store: store, logger: logger}
	if err := registry.Register(dedupeMiddlewareType, middleware.wrap); err != nil {
		logger.Error("register vale dedupe middleware", "error", err)
	}
	return registry
}

func dedupeMiddlewareName(upstreamID string) string {
	return dedupeMiddlewareNamePrefix + strings.TrimSpace(upstreamID)
}

func upstreamFromDedupeMiddlewareName(name string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(name), dedupeMiddlewareNamePrefix))
}

func (m *dedupeMiddleware) wrap(next http.Handler, middleware valeruntime.MiddlewareRuntime) http.Handler {
	upstreamID := upstreamFromDedupeMiddlewareName(middleware.Name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m == nil || m.store == nil || upstreamID == "" {
			next.ServeHTTP(w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			m.handlePut(next, upstreamID, w, r)
		case http.MethodDelete:
			m.handleDelete(next, w, r)
		case http.MethodGet, http.MethodHead:
			m.handleRead(next, upstreamID, w, r)
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (m *dedupeMiddleware) handlePut(next http.Handler, upstreamID string, w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bucket, key, ok := parseS3ObjectPath(r.URL.Path)
	if !ok {
		next.ServeHTTP(w, r)
		return
	}
	captured, ok := m.capturePutBody(ctx, w, r, bucket, key)
	if !ok {
		return
	}
	defer cleanupCapturedBody(ctx, m.logger, captured)

	ref, reusable, err := m.reusableDigestRef(ctx, captured.digest, upstreamID)
	if err != nil {
		writeS3ProxyInternalError(w, "failed to lookup object digest")
		m.logger.ErrorContext(ctx, "lookup reusable digest ref", "bucket", bucket, "key", key, "error", err)
		return
	}
	if reusable {
		m.handlePutDedupeHit(w, r, bucket, key, captured, ref)
		return
	}
	m.handlePutMiss(next, upstreamID, w, r, bucket, key, captured)
}

func (m *dedupeMiddleware) capturePutBody(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	bucket string,
	key string,
) (*capturedRequestBody, bool) {
	captured, err := captureRequestBody(r.Body)
	if err != nil {
		writeS3ProxyInternalError(w, "failed to read request body")
		m.logger.ErrorContext(ctx, "capture s3 put request body", "bucket", bucket, "key", key, "error", err)
		return nil, false
	}
	return captured, true
}

func (m *dedupeMiddleware) handlePutDedupeHit(
	w http.ResponseWriter,
	r *http.Request,
	bucket string,
	key string,
	captured *capturedRequestBody,
	ref model.DigestRef,
) {
	event := newObjectPutEventFromDigestHit(r, bucket, key, captured, ref)
	version, err := m.commitObjectPut(r.Context(), event)
	if err != nil {
		writeS3ProxyInternalError(w, "failed to commit deduplicated object metadata")
		m.logger.ErrorContext(r.Context(), "commit deduplicated s3 put metadata", "bucket", bucket, "key", key, "error", err)
		return
	}
	w.Header().Set("ETag", event.ETag)
	w.Header().Set("x-amz-version-id", version.VersionID)
	w.Header().Set("X-Maxio-Dedupe", "hit")
	w.Header().Set("X-Maxio-Canonical-Bucket", ref.UpstreamBucket)
	w.Header().Set("X-Maxio-Canonical-Key", ref.UpstreamKey)
	w.WriteHeader(http.StatusOK)
	m.publishObjectPut(r.Context(), event)
}

func (m *dedupeMiddleware) handlePutMiss(
	next http.Handler,
	upstreamID string,
	w http.ResponseWriter,
	r *http.Request,
	bucket string,
	key string,
	captured *capturedRequestBody,
) {
	body, ok := m.openCapturedPutBody(w, r, bucket, key, captured)
	if !ok {
		return
	}
	defer closeReplayedPutBody(r.Context(), m.logger, body)

	r.Body = body
	r.ContentLength = captured.size
	response := newProxyResponseBuffer()
	next.ServeHTTP(response, r)
	if !isSuccessfulProxyStatus(response.statusCode()) {
		m.sendBufferedPutResponse(r.Context(), w, response, bucket, key, false)
		return
	}
	event := newObjectPutEventFromUpstreamResponse(r, upstreamID, bucket, key, captured, response)
	version, err := m.commitObjectPut(r.Context(), event)
	if err != nil {
		writeS3ProxyInternalError(w, "failed to commit object metadata")
		m.logger.ErrorContext(r.Context(), "commit s3 put metadata", "bucket", bucket, "key", key, "error", err)
		return
	}
	response.Header().Set("x-amz-version-id", version.VersionID)
	response.Header().Set("X-Maxio-Dedupe", "miss")
	if response.Header().Get("ETag") == "" {
		response.Header().Set("ETag", digestETag(captured.digest))
	}
	if m.sendBufferedPutResponse(r.Context(), w, response, bucket, key, true) {
		m.publishObjectPut(r.Context(), event)
	}
}

func (m *dedupeMiddleware) openCapturedPutBody(
	w http.ResponseWriter,
	r *http.Request,
	bucket string,
	key string,
	captured *capturedRequestBody,
) (io.ReadCloser, bool) {
	body, err := captured.Open()
	if err != nil {
		writeS3ProxyInternalError(w, "failed to replay request body")
		m.logger.ErrorContext(r.Context(), "open captured s3 put request body", "bucket", bucket, "key", key, "error", err)
		return nil, false
	}
	return body, true
}

func (m *dedupeMiddleware) sendBufferedPutResponse(
	ctx context.Context,
	w http.ResponseWriter,
	response *proxyResponseBuffer,
	bucket string,
	key string,
	success bool,
) bool {
	if err := response.Send(w); err != nil {
		m.logger.ErrorContext(ctx, "send s3 put response", "bucket", bucket, "key", key, "success", success, "error", err)
		return false
	}
	return true
}
