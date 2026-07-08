package handler

import (
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/processing"
	"log/slog"
	"testing"
)

func TestProcessingRecordResponseSortsProcessorResults(t *testing.T) {
	service := NewService(
		newDependenciesWithProcessing(metadata.NewInMemoryMetadata(), nil, nil),
		slog.New(slog.DiscardHandler),
		config.Config{},
	)
	response := service.processingRecordToResponse(processing.Record{
		Object: processing.ObjectRef{Bucket: "docs", Key: "sorted.txt", VersionID: "v1"},
		Status: processing.StatusSucceeded,
		Results: collectionlist.NewList(
			processing.ProcessorResult{Processor: "tika", Mode: processing.ModeAsyncPermissive, Status: processing.StatusSucceeded},
			processing.ProcessorResult{Processor: "clamav", Mode: processing.ModeInlineStrict, Status: processing.StatusSucceeded},
		),
	})
	if len(response.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(response.Results))
	}
	if response.Results[0].Processor != "clamav" || response.Results[1].Processor != "tika" {
		t.Fatalf("results = %#v, want sorted by processor", response.Results)
	}
}
