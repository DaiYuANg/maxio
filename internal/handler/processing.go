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
		s.writeJSON(w, http.StatusOK, emptyProcessingStatusResponse())
		return
	}
	s.writeJSON(w, http.StatusOK, processingStatusFromSnapshot(s.processing.Snapshot()))
}

func emptyProcessingStatusResponse() processingStatusResponse {
	return processingStatusResponse{
		Mode:              processing.ModeDisabled,
		Timeout:           (0 * time.Second).String(),
		Processors:        []string{},
		ProcessorModes:    map[string]string{},
		ProcessorFailOpen: map[string]bool{},
		Capabilities:      []string{},
	}
}

func processingStatusFromSnapshot(snapshot processing.Snapshot) processingStatusResponse {
	return processingStatusResponse{
		Enabled:           snapshot.Enabled,
		Mode:              snapshot.Mode,
		FailOpen:          snapshot.FailOpen,
		Timeout:           snapshot.Timeout.Round(time.Millisecond).String(),
		Processors:        stringListValues(snapshot.Processors),
		ProcessorModes:    snapshot.ProcessorModes,
		ProcessorFailOpen: snapshot.ProcessorFailOpen,
		Capabilities:      stringListValues(snapshot.Capabilities),
	}
}

func stringListValues[T ~string](values interface{ Values() []T }) []string {
	if values == nil {
		return []string{}
	}
	items := values.Values()
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, string(item))
	}
	return result
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
	if s.handleProcessingRecordStatusQuery(w, r) {
		return
	}
	s.handleProcessingRecordLookup(w, r)
}

func (s *Service) handleProcessingRecordStatusQuery(w http.ResponseWriter, r *http.Request) bool {
	status := processing.NormalizeStatus(r.URL.Query().Get("status"))
	if status == "" {
		return false
	}
	if !processing.ValidStatus(status) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be one of skipped, queued, running, succeeded, failed, blocked"})
		return true
	}
	if processingRecordHasIdentityParams(r) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status cannot be combined with bucket, key, version_id, or digest"})
		return true
	}
	s.handleProcessingRecordList(w, r, status)
	return true
}

func (s *Service) handleProcessingRecordLookup(w http.ResponseWriter, r *http.Request) {
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
		Results:         sortedProcessingResults(record.Results),
	}
}

func sortedProcessingResults(results interface {
	Values() []processing.ProcessorResult
}) []processing.ProcessorResult {
	if results == nil {
		return []processing.ProcessorResult{}
	}
	items := append([]processing.ProcessorResult{}, results.Values()...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].Processor == items[j].Processor {
			return items[i].Mode < items[j].Mode
		}
		return items[i].Processor < items[j].Processor
	})
	return items
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
