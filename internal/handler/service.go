package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/dedupe"
	"github.com/lyonbrown4d/maxio/internal/object"
)

const defaultSearchPath = "/_search"
const defaultClusterMembersPath = "/_cluster/members"
const defaultClusterBootstrapPath = "/_cluster/bootstrap"
const defaultClusterJoinPath = "/_cluster/join"
const defaultClusterStatusPath = "/_cluster/status"
const defaultClusterNodesPath = "/_cluster/nodes"
const defaultClusterReconcilePath = "/_cluster/reconcile"
const defaultDiscoveryPath = "/_cluster/discovery"
const defaultDedupeStatusPath = "/_dedupe/status"
const defaultDedupePlanPath = "/_dedupe/plan"
const defaultDedupeRunPath = "/_dedupe/run"
const defaultIndexStatusPath = "/_index/status"
const defaultIndexRebuildPath = "/_index/rebuild"

type Service struct {
	logger *slog.Logger
	cfg    config.Config
	dedupe *dedupe.Runtime
	http   *httpRequestMetrics
	Dependencies
}

func NewService(deps Dependencies, logger *slog.Logger, cfg config.Config) *Service {
	return newService(deps, logger, cfg, nil)
}

func newService(deps Dependencies, logger *slog.Logger, cfg config.Config, dedupeRuntime *dedupe.Runtime) *Service {
	return &Service{
		logger:       logger,
		cfg:          cfg,
		dedupe:       dedupeRuntime,
		http:         newHTTPRequestMetrics(),
		Dependencies: deps,
	}
}

func (s *Service) RegisterHTTP(router *http.ServeMux) {
	router.HandleFunc("/", s.serveHTTP)
}

func (s *Service) serveHTTP(w http.ResponseWriter, r *http.Request) {
	s.beginHTTPRequest()
	recorder := newStatusResponseWriter(w)
	requestID := requestIDFromRequest(r)
	recorder.Header().Set(requestIDHeader, requestID)
	r = r.WithContext(contextWithRequestID(r.Context(), requestID))
	startedAt := time.Now()
	defer func() {
		s.recordHTTPRequest(r, recorder.status(), time.Since(startedAt))
	}()
	s.dispatchHTTP(recorder, r)
}

func (s *Service) dispatchHTTP(w http.ResponseWriter, r *http.Request) {
	route := strings.Trim(path.Clean(r.URL.Path), "/")
	parts := strings.Split(route, "/")
	if !s.authorizeControlHTTPRequest(w, r, route, parts) {
		return
	}
	if s.handleControlRoute(w, r, route, parts) {
		return
	}
	if !s.authorizeNativeObjectHTTPRequest(w, r, route, parts) {
		return
	}
	if !s.cfg.EnableNativeObjectAPI {
		s.writeS3ProxyNotImplemented(w)
		return
	}

	if route == "" {
		s.handleBuckets(w, r)
		return
	}

	if len(parts) == 1 {
		s.handleBucket(w, r, parts[0])
		return
	}

	bucket := parts[0]
	key := strings.Join(parts[1:], "/")
	s.handleObject(w, r, bucket, key)
}

func (s *Service) handleBuckets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.objects == nil {
		s.writeLegacyObjectUnavailable(w)
		return
	}
	buckets, err := s.objects.ListBuckets(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, buckets)
}

func (s *Service) handleBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if s.objects == nil {
		s.writeLegacyObjectUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		prefix := r.URL.Query().Get("prefix")
		items, err := s.objects.ListObjects(r.Context(), bucket, prefix)
		if errors.Is(err, object.ErrBucketNotFound) {
			s.writeError(w, err)
			return
		}
		if err != nil {
			s.writeError(w, err)
			return
		}
		s.writeJSON(w, http.StatusOK, items)
	case http.MethodPut:
		if err := s.objects.CreateBucket(r.Context(), bucket); err != nil {
			s.writeError(w, err)
			return
		}
		s.auditHTTP(r, "bucket.create", "bucket", bucket)
		s.writeJSON(w, http.StatusCreated, map[string]string{"bucket": bucket, "status": "created"})
	case http.MethodDelete:
		if err := s.objects.DeleteBucket(r.Context(), bucket); err != nil {
			s.writeError(w, err)
			return
		}
		s.auditHTTP(r, "bucket.delete", "bucket", bucket)
		s.writeJSON(w, http.StatusNoContent, nil)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if s.objects == nil {
		s.writeLegacyObjectUnavailable(w)
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleGetObject(w, r, bucket, key)
	case http.MethodHead:
		s.handleHeadObject(w, r, bucket, key)
	case http.MethodPut:
		s.handlePutObject(w, r, bucket, key)
	case http.MethodDelete:
		s.handleDeleteObject(w, r, bucket, key)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleGetObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	body, meta, err := s.objects.GetObject(r.Context(), bucket, key)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeGetObjectResponse(w, r, body, meta)
}

func (s *Service) handleHeadObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	meta, err := s.objects.StatObject(r.Context(), bucket, key)
	if err != nil {
		s.writeError(w, err)
		return
	}
	writeObjectHeaders(w, meta)
	w.WriteHeader(http.StatusOK)
}

func (s *Service) handlePutObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	meta, err := s.objects.PutObject(r.Context(), bucket, key, r.Body, object.PutOptions{
		ContentType:        r.Header.Get("Content-Type"),
		CacheControl:       r.Header.Get("Cache-Control"),
		ContentDisposition: r.Header.Get("Content-Disposition"),
		ContentEncoding:    r.Header.Get("Content-Encoding"),
		ContentLanguage:    r.Header.Get("Content-Language"),
	})
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "object.put",
		"bucket", bucket,
		"key", key,
		"size", meta.Size,
		"etag", meta.ETag,
	)
	s.writeJSON(w, http.StatusOK, meta)
}

func (s *Service) handleDeleteObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	_, err := s.objects.DeleteObject(r.Context(), bucket, key)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.auditHTTP(r, "object.delete", "bucket", bucket, "key", key)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.objects == nil {
		s.writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "search is not wired to the S3 proxy metadata repository yet"})
		return
	}

	query := object.SearchQuery{}
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
			s.writeError(w, err)
			return
		}
	} else {
		query.Query = r.URL.Query().Get("q")
		query.Bucket = r.URL.Query().Get("bucket")
		query.Prefix = r.URL.Query().Get("prefix")
		query.NameContains = r.URL.Query().Get("name_contains")
		query.ContentType = r.URL.Query().Get("content_type")
	}

	result, err := s.objects.Search(r.Context(), query)
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, result)
}

func contentTypeOrDefault(v string) string {
	if v == "" {
		return "application/octet-stream"
	}
	return v
}

func formatInt(v int64) string {
	return strconv.FormatInt(v, 10)
}

func writeObjectHeaders(w http.ResponseWriter, meta object.ObjectMeta) {
	w.Header().Set("ETag", meta.ETag)
	w.Header().Set("Content-Type", contentTypeOrDefault(meta.ContentType))
	w.Header().Set("Content-Length", formatInt(meta.Size))
	w.Header().Set("Accept-Ranges", "bytes")
	setObjectHeaderIfNotEmpty(w, "Cache-Control", meta.CacheControl)
	setObjectHeaderIfNotEmpty(w, "Content-Disposition", meta.ContentDisposition)
	setObjectHeaderIfNotEmpty(w, "Content-Encoding", meta.ContentEncoding)
	setObjectHeaderIfNotEmpty(w, "Content-Language", meta.ContentLanguage)
	for key, value := range meta.UserMetadata {
		setObjectHeaderIfNotEmpty(w, "x-amz-meta-"+strings.ToLower(key), value)
	}
}

func setObjectHeaderIfNotEmpty(w http.ResponseWriter, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	w.Header().Set(key, value)
}
