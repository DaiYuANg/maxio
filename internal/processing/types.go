// Package processing defines the optional object processing pipeline.
package processing

import (
	"context"
	"errors"
	"io"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const (
	ModeDisabled        = "disabled"
	ModeAsyncPermissive = "async_permissive"
	ModeAsyncStrict     = "async_strict"
	ModeInlineStrict    = "inline_strict"
)

const (
	StatusSkipped   = "skipped"
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusBlocked   = "blocked"
)

const (
	CapabilityAntivirus        Capability = "antivirus"
	CapabilityTextExtraction   Capability = "text_extraction"
	CapabilityMetadataExtract  Capability = "metadata_extract"
	CapabilityContentClassify  Capability = "content_classify"
	CapabilityPolicyEvaluation Capability = "policy_evaluation"
)

var (
	ErrProcessingPending = errors.New("object processing pending")
	ErrProcessingFailed  = errors.New("object processing failed")
	ErrProcessingDenied  = errors.New("object processing denied")
)

type Capability string

type Config struct {
	Enabled  bool
	Mode     string
	Timeout  time.Duration
	FailOpen bool
}

type ObjectRef struct {
	Bucket         string
	Key            string
	VersionID      string
	Digest         string
	ETag           string
	Size           int64
	ContentType    string
	UpstreamID     string
	UpstreamBucket string
	UpstreamKey    string
	UserMetadata   map[string]string
}

type Input struct {
	Object      ObjectRef
	OpenContent func(context.Context) (io.ReadCloser, error)
	Cleanup     func(context.Context)
}

type ProcessorResult struct {
	Processor   string            `json:"processor"`
	Mode        string            `json:"mode,omitempty"`
	Status      string            `json:"status"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CompletedAt time.Time         `json:"completed_at,omitempty"`
}

type Record struct {
	Object    ObjectRef                             `json:"object"`
	Mode      string                                `json:"mode"`
	Status    string                                `json:"status"`
	Error     string                                `json:"error,omitempty"`
	Results   *collectionlist.List[ProcessorResult] `json:"results,omitempty"`
	UpdatedAt time.Time                             `json:"updated_at"`
}

type ReadDecision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

type Snapshot struct {
	Enabled           bool                         `json:"enabled"`
	Mode              string                       `json:"mode"`
	FailOpen          bool                         `json:"fail_open"`
	Timeout           time.Duration                `json:"timeout"`
	Processors        *collectionlist.List[string] `json:"processors"`
	ProcessorModes    map[string]string            `json:"processor_modes"`
	ProcessorFailOpen map[string]bool              `json:"processor_fail_open"`
	Capabilities      *collectionlist.List[string] `json:"capabilities"`
}

type Processor interface {
	Name() string
	Capabilities() *collectionset.Set[Capability]
	Process(ctx context.Context, input Input) (ProcessorResult, error)
}

type ProcessorFailOpenProvider interface {
	FailOpen() bool
}

type ProcessorBinding struct {
	Processor Processor
	Mode      string
}

func BindProcessor(processor Processor, mode string) ProcessorBinding {
	return ProcessorBinding{Processor: processor, Mode: mode}
}

type RecordStore interface {
	UpsertProcessingRecord(ctx context.Context, record model.ProcessingRecord) (model.ProcessingRecord, error)
	GetProcessingRecord(ctx context.Context, bucket, key, versionID, digest string) (model.ProcessingRecord, bool, error)
	ListProcessingRecords(ctx context.Context, status string, limit int) (*collectionlist.List[model.ProcessingRecord], error)
	DeleteProcessingRecord(ctx context.Context, bucket, key, versionID, digest string) (bool, error)
}
