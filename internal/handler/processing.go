package handler

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/processing"
)

type processingStatusResponse struct {
	Enabled           bool              `json:"enabled"`
	Mode              string            `json:"mode"`
	FailOpen          bool              `json:"fail_open"`
	Timeout           string            `json:"timeout"`
	Processors        []string          `json:"processors"`
	ProcessorModes    map[string]string `json:"processor_modes"`
	ProcessorFailOpen map[string]bool   `json:"processor_fail_open"`
	Capabilities      []string          `json:"capabilities"`
}

type processingRecordResponse struct {
	Bucket          string                       `json:"bucket"`
	Key             string                       `json:"key"`
	VersionID       string                       `json:"version_id,omitempty"`
	Digest          string                       `json:"digest,omitempty"`
	Mode            string                       `json:"mode"`
	Status          string                       `json:"status"`
	Error           string                       `json:"error,omitempty"`
	ReadAllowed     bool                         `json:"read_allowed"`
	ReadBlockReason string                       `json:"read_block_reason,omitempty"`
	UpdatedAt       time.Time                    `json:"updated_at"`
	Results         []processing.ProcessorResult `json:"results,omitempty"`
}

type processingRecordsResponse struct {
	Records []processingRecordResponse `json:"records"`
}

func (s *Service) handleProcessingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s == nil || s.processing == nil {
		s.writeJSON(w, http.StatusOK, processingStatusResponse{
			Mode:              processing.ModeDisabled,
			Timeout:           (0 * time.Second).String(),
			Processors:        []string{},
			ProcessorModes:    map[string]string{},
			ProcessorFailOpen: map[string]bool{},
			Capabilities:      []string{},
		})
		return
	}
	snapshot := s.processing.Snapshot()
	processors := []string{}
	if snapshot.Processors != nil {
		processors = snapshot.Processors.Values()
	}
	capabilities := []string{}
	if snapshot.Capabilities != nil {
		capabilities = snapshot.Capabilities.Values()
	}
	s.writeJSON(w, http.StatusOK, processingStatusResponse{
		Enabled:           snapshot.Enabled,
		Mode:              snapshot.Mode,
		FailOpen:          snapshot.FailOpen,
		Timeout:           snapshot.Timeout.Round(time.Millisecond).String(),
		Processors:        processors,
		ProcessorModes:    snapshot.ProcessorModes,
		ProcessorFailOpen: snapshot.ProcessorFailOpen,
		Capabilities:      capabilities,
	})
}

func (s *Service) handleProcessingRecord(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s == nil || s.processing == nil {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "processing record not found"})
		return
	}
	status := processing.NormalizeStatus(r.URL.Query().Get("status"))
	if status != "" {
		if !processing.ValidStatus(status) {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be one of skipped, queued, running, succeeded, failed, blocked"})
			return
		}
		if processingRecordHasIdentityParams(r) {
			s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status cannot be combined with bucket, key, version_id, or digest"})
			return
		}
		s.handleProcessingRecordList(w, r, status)
		return
	}
	object, ok := processingRecordObjectFromRequest(r)
	if !ok {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bucket, key, and version_id or digest are required"})
		return
	}
	record, found, err := s.processing.LookupRecord(r.Context(), object)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "processing record lookup failed"})
		return
	}
	if !found {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "processing record not found"})
		return
	}
	s.writeJSON(w, http.StatusOK, s.processingRecordToResponse(record))
}

func (s *Service) handleProcessingRecordList(w http.ResponseWriter, r *http.Request, status string) {
	records, err := s.processing.ListRecords(r.Context(), status, processingRecordLimitFromRequest(r))
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "processing records lookup failed"})
		return
	}
	responses := []processingRecordResponse{}
	if records != nil {
		responses = make([]processingRecordResponse, 0, records.Len())
		records.Range(func(_ int, record processing.Record) bool {
			responses = append(responses, s.processingRecordToResponse(record))
			return true
		})
	}
	s.writeJSON(w, http.StatusOK, processingRecordsResponse{Records: responses})
}

func (s *Service) processingRecordToResponse(record processing.Record) processingRecordResponse {
	results := []processing.ProcessorResult{}
	if record.Results != nil {
		results = append(results, record.Results.Values()...)
		sort.Slice(results, func(i, j int) bool {
			if results[i].Processor == results[j].Processor {
				return results[i].Mode < results[j].Mode
			}
			return results[i].Processor < results[j].Processor
		})
	}
	decision := processing.ReadDecision{Allowed: true}
	if s != nil && s.processing != nil {
		decision = s.processing.ReadDecision(record)
	}
	return processingRecordResponse{
		Bucket:          record.Object.Bucket,
		Key:             record.Object.Key,
		VersionID:       record.Object.VersionID,
		Digest:          record.Object.Digest,
		Mode:            record.Mode,
		Status:          record.Status,
		Error:           record.Error,
		ReadAllowed:     decision.Allowed,
		ReadBlockReason: decision.Reason,
		UpdatedAt:       record.UpdatedAt,
		Results:         results,
	}
}

func processingRecordHasIdentityParams(r *http.Request) bool {
	query := r.URL.Query()
	return strings.TrimSpace(query.Get("bucket")) != "" || query.Get("key") != "" || strings.TrimSpace(query.Get("version_id")) != "" || strings.TrimSpace(query.Get("digest")) != ""
}

func processingRecordLimitFromRequest(r *http.Request) int {
	limit, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	if err != nil {
		return 100
	}
	if limit <= 0 {
		return 100
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func processingRecordObjectFromRequest(r *http.Request) (processing.ObjectRef, bool) {
	query := r.URL.Query()
	object := processing.ObjectRef{
		Bucket:    strings.TrimSpace(query.Get("bucket")),
		Key:       query.Get("key"),
		VersionID: strings.TrimSpace(query.Get("version_id")),
		Digest:    strings.TrimSpace(query.Get("digest")),
	}
	return object, object.Bucket != "" && object.Key != "" && (object.VersionID != "" || object.Digest != "")
}
