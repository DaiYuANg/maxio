package store

import (
	"time"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/model"
)

const maxDedupeIssues = 50

const (
	DedupeIssueRefCountDrift    = "ref_count_drift"
	DedupeIssueOrphanBlobRef    = "orphan_blob_ref"
	DedupeIssueMissingBlobRef   = "missing_blob_ref"
	DedupeIssueLayoutMismatch   = "layout_mismatch"
	DedupeIssueLayoutReadFailed = "layout_read_failed"
)

type DedupeOptions struct {
	DryRun   bool
	MaxFixes int
}

type DedupeResult struct {
	GeneratedAt           time.Time     `json:"generated_at"`
	DryRun                bool          `json:"dry_run"`
	Buckets               int           `json:"buckets"`
	Objects               int           `json:"objects"`
	BlobRefs              int           `json:"blob_refs"`
	Hashes                int           `json:"hashes"`
	Fixes                 int           `json:"fixes"`
	RefCountDrift         int           `json:"ref_count_drift"`
	RefCountIncreased     int           `json:"ref_count_increased"`
	RefCountDecreased     int           `json:"ref_count_decreased"`
	MissingBlobRefs       int           `json:"missing_blob_refs"`
	MissingBlobRefsFixed  int           `json:"missing_blob_refs_fixed"`
	OrphanBlobRefs        int           `json:"orphan_blob_refs"`
	OrphanBlobRefsRemoved int           `json:"orphan_blob_refs_removed"`
	LayoutsMismatched     int           `json:"layouts_mismatched"`
	LayoutsCanonicalized  int           `json:"layouts_canonicalized"`
	BytesReclaimable      int64         `json:"bytes_reclaimable"`
	BytesReclaimed        int64         `json:"bytes_reclaimed"`
	Limited               bool          `json:"limited"`
	Issues                []DedupeIssue `json:"issues,omitempty"`
}

type DedupeIssue struct {
	Kind             string `json:"kind"`
	Hash             string `json:"hash,omitempty"`
	Bucket           string `json:"bucket,omitempty"`
	Key              string `json:"key,omitempty"`
	Reason           string `json:"reason,omitempty"`
	ExpectedRefCount int    `json:"expected_ref_count,omitempty"`
	ActualRefCount   int    `json:"actual_ref_count,omitempty"`
	Path             string `json:"path,omitempty"`
	CanonicalPath    string `json:"canonical_path,omitempty"`
	Size             int64  `json:"size,omitempty"`
}

type dedupeHashStat struct {
	count int
	size  int64
	first dedupeObject
}

type dedupeObject struct {
	meta    model.ObjectMeta
	info    engine.ObjectInfo
	hasInfo bool
	err     error
}

func newDedupeResult(opts DedupeOptions) DedupeResult {
	return DedupeResult{
		GeneratedAt: time.Now().UTC(),
		DryRun:      opts.DryRun,
	}
}

func (result *DedupeResult) addIssue(issue DedupeIssue) {
	if result == nil || len(result.Issues) >= maxDedupeIssues {
		return
	}
	result.Issues = append(result.Issues, issue)
}

func (result *DedupeResult) reachedLimit(opts DedupeOptions) bool {
	if result == nil || opts.MaxFixes <= 0 || result.Fixes < opts.MaxFixes {
		return false
	}
	result.Limited = true
	return true
}
