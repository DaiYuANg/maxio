package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/dedupe"
	"github.com/lyonbrown4d/maxio/internal/object"
)

const defaultSearchPath = "/_search"
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
	if !s.cfg.EnableS3Proxy {
		s.writeS3ProxyNotImplemented(w)
		return
	}
	http.NotFound(w, r)
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
