package metadata

import (
	"context"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func TestInMemoryProcessingRecordLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMetadata()
	record := model.ProcessingRecord{
		Bucket:    "docs",
		Key:       "a.txt",
		VersionID: "v1",
		Mode:      "inline_strict",
		Status:    "succeeded",
		Results:   `[{"processor":"noop","status":"succeeded"}]`,
	}
	stored, err := store.UpsertProcessingRecord(ctx, record)
	if err != nil {
		t.Fatalf("upsert processing record: %v", err)
	}
	if stored.ID == "" {
		t.Fatal("expected processing record id")
	}
	got, found, err := store.GetProcessingRecord(ctx, "docs", "a.txt", "v1", "")
	if err != nil {
		t.Fatalf("get processing record: %v", err)
	}
	if !found {
		t.Fatal("expected processing record")
	}
	if got.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", got.Status)
	}
	deleted, err := store.DeleteProcessingRecord(ctx, "docs", "a.txt", "v1", "")
	if err != nil {
		t.Fatalf("delete processing record: %v", err)
	}
	if !deleted {
		t.Fatal("expected processing record delete")
	}
	_, found, err = store.GetProcessingRecord(ctx, "docs", "a.txt", "v1", "")
	if err != nil {
		t.Fatalf("get deleted processing record: %v", err)
	}
	if found {
		t.Fatal("did not expect deleted processing record")
	}
}
func TestInMemoryProcessingRecordKeepsObjectKeyWhitespaceDistinct(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMetadata()

	spaced := model.ProcessingRecord{Bucket: "docs", Key: " file.txt", VersionID: "v1", Mode: "inline_strict", Status: "succeeded"}
	plain := model.ProcessingRecord{Bucket: "docs", Key: "file.txt", VersionID: "v1", Mode: "inline_strict", Status: "blocked"}
	if _, err := store.UpsertProcessingRecord(ctx, spaced); err != nil {
		t.Fatalf("upsert spaced processing record: %v", err)
	}
	if _, err := store.UpsertProcessingRecord(ctx, plain); err != nil {
		t.Fatalf("upsert plain processing record: %v", err)
	}

	gotSpaced, found, err := store.GetProcessingRecord(ctx, "docs", " file.txt", "v1", "")
	if err != nil {
		t.Fatalf("get spaced processing record: %v", err)
	}
	if !found || gotSpaced.Status != "succeeded" {
		t.Fatalf("spaced status = %q, found = %v", gotSpaced.Status, found)
	}
	gotPlain, found, err := store.GetProcessingRecord(ctx, "docs", "file.txt", "v1", "")
	if err != nil {
		t.Fatalf("get plain processing record: %v", err)
	}
	if !found || gotPlain.Status != "blocked" {
		t.Fatalf("plain status = %q, found = %v", gotPlain.Status, found)
	}
}

func TestProcessingRecordIDUsesDigestWhenVersionIDIsEmpty(t *testing.T) {
	withVersion := processingRecordID("docs", "a.txt", "v1", "sha256:abc")
	withDigest := processingRecordID("docs", "a.txt", "", "sha256:abc")
	if withVersion == "" {
		t.Fatal("expected version processing record id")
	}
	if withDigest == "" {
		t.Fatal("expected digest processing record id")
	}
	if withVersion == withDigest {
		t.Fatal("expected version and digest processing record ids to be distinct")
	}
}

func TestProcessingRecordIDIsURLSafeAndDelimiterSafe(t *testing.T) {
	id := processingRecordID("docs", "a/b c.txt", "v1", "")
	if id == "" {
		t.Fatal("expected processing record id")
	}
	if strings.ContainsAny(id, "\x00/+=") {
		t.Fatalf("processing record id %q contains unsafe separator characters", id)
	}
	if strings.Count(id, ".") != 2 {
		t.Fatalf("processing record id %q should use two dot separators", id)
	}
}
func TestInMemoryListProcessingRecordsByStatus(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMetadata()
	if _, err := store.UpsertProcessingRecord(ctx, model.ProcessingRecord{Bucket: "docs", Key: "ok.txt", VersionID: "v1", Mode: "inline_strict", Status: "succeeded"}); err != nil {
		t.Fatalf("upsert succeeded processing record: %v", err)
	}
	if _, err := store.UpsertProcessingRecord(ctx, model.ProcessingRecord{Bucket: "docs", Key: "bad.txt", VersionID: "v1", Mode: "inline_strict", Status: "blocked"}); err != nil {
		t.Fatalf("upsert blocked processing record: %v", err)
	}

	records, err := store.ListProcessingRecords(ctx, "blocked", 10)
	if err != nil {
		t.Fatalf("list processing records: %v", err)
	}
	if records.Len() != 1 {
		t.Fatalf("records len = %d, want 1", records.Len())
	}
	if records.Values()[0].Key != "bad.txt" {
		t.Fatalf("record key = %q, want bad.txt", records.Values()[0].Key)
	}
}
func TestProcessingRecordStatusIsNormalized(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMetadata()
	if _, err := store.UpsertProcessingRecord(ctx, model.ProcessingRecord{Bucket: "docs", Key: "case.txt", VersionID: "v1", Mode: " Inline_Strict ", Status: " Succeeded "}); err != nil {
		t.Fatalf("upsert processing record: %v", err)
	}
	record, found, err := store.GetProcessingRecord(ctx, "docs", "case.txt", "v1", "")
	if err != nil {
		t.Fatalf("get processing record: %v", err)
	}
	if !found {
		t.Fatal("expected processing record")
	}
	if record.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", record.Status)
	}
	if record.Mode != "inline_strict" {
		t.Fatalf("mode = %q, want inline_strict", record.Mode)
	}
}
