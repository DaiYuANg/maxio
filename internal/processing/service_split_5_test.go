package processing

import (
	"context"
	"fmt"
	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
	"log/slog"
	"testing"
	"time"
)

func TestDigestDiscardDoesNotTombstoneFuturePreflight(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeInlineStrict, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeInlineStrict),
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "retry.txt", Digest: "sha256:retry"}
	service.Discard(ctx, object)
	if err := service.ProcessBeforeCommit(ctx, Input{Object: object}); err != nil {
		t.Fatalf("preflight after digest discard: %v", err)
	}
	if _, found := service.Record(ctx, object); !found {
		t.Fatal("expected digest preflight record after retry")
	}
}

func TestDiscardHidesExistingVersionRecord(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeAsyncPermissive),
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "discarded.txt", VersionID: "v1"}
	if err := service.storeRecord(ctx, object, StatusSucceeded, "", collectionlist.NewList[ProcessorResult]()); err != nil {
		t.Fatalf("store processing record: %v", err)
	}
	service.Discard(ctx, object)
	if _, found := service.Record(ctx, object); found {
		t.Fatal("expected discarded version record to be hidden")
	}
}

func TestDiscardTombstonesAreBounded(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeAsyncPermissive),
	)
	ctx := context.Background()
	for i := range maxDiscardedTombstones + 1 {
		service.Discard(ctx, ObjectRef{Bucket: "docs", Key: "bounded.txt", VersionID: fmt.Sprintf("v%d", i)})
	}
	if len(service.discarded) != maxDiscardedTombstones {
		t.Fatalf("discarded len = %d, want %d", len(service.discarded), maxDiscardedTombstones)
	}
	first := ObjectRef{Bucket: "docs", Key: "bounded.txt", VersionID: "v0"}
	if service.isDiscarded(first) {
		t.Fatal("expected oldest tombstone to be evicted")
	}
	last := ObjectRef{Bucket: "docs", Key: "bounded.txt", VersionID: fmt.Sprintf("v%d", maxDiscardedTombstones)}
	if !service.isDiscarded(last) {
		t.Fatal("expected newest tombstone to be retained")
	}
}

func TestDiscardTombstoneDuplicateDoesNotGrowOrder(t *testing.T) {
	service := NewServiceWithBindings(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		nil,
		BindProcessor(NewNoopProcessor(), ModeAsyncPermissive),
	)
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "duplicate-discard.txt", VersionID: "v1"}
	service.Discard(ctx, object)
	service.Discard(ctx, object)
	if len(service.discardedOrder) != 1 {
		t.Fatalf("discarded order len = %d, want 1", len(service.discardedOrder))
	}
}

type hookRecordStore struct {
	record   model.ProcessingRecord
	records  *collectionlist.List[model.ProcessingRecord]
	onUpsert func()
	onGet    func()
	onDelete func()
}

func (s hookRecordStore) UpsertProcessingRecord(_ context.Context, record model.ProcessingRecord) (model.ProcessingRecord, error) {
	if s.onUpsert != nil {
		s.onUpsert()
	}
	return record, nil
}

func (s hookRecordStore) GetProcessingRecord(context.Context, string, string, string, string) (model.ProcessingRecord, bool, error) {
	if s.onGet != nil {
		s.onGet()
	}
	return s.record, true, nil
}

func (s hookRecordStore) ListProcessingRecords(_ context.Context, _ string, limit int) (*collectionlist.List[model.ProcessingRecord], error) {
	if s.records == nil {
		return collectionlist.NewList[model.ProcessingRecord](), nil
	}
	if limit <= 0 || s.records.Len() <= limit {
		return s.records, nil
	}
	return collectionlist.NewList(s.records.Values()[:limit]...), nil
}

func (s hookRecordStore) DeleteProcessingRecord(context.Context, string, string, string, string) (bool, error) {
	if s.onDelete != nil {
		s.onDelete()
	}
	return true, nil
}

func TestLookupRecordRechecksDiscardAfterStoreRead(t *testing.T) {
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "discard-race.txt", VersionID: "v1"}
	var service *Service
	service = NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{
			record: model.ProcessingRecord{Bucket: object.Bucket, Key: object.Key, VersionID: object.VersionID, Mode: ModeAsyncPermissive, Status: StatusSucceeded},
			onGet: func() {
				service.Discard(ctx, object)
			},
		},
	)
	record, found, err := service.LookupRecord(ctx, object)
	if err != nil {
		t.Fatalf("lookup record: %v", err)
	}
	if found {
		t.Fatalf("record = %#v, want hidden after discard", record)
	}
}

func TestVersionDiscardDoesNotHoldLockDuringStoreDelete(t *testing.T) {
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "discard-lock.txt", VersionID: "v1"}
	done := make(chan struct{})
	var service *Service
	service = NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{onDelete: func() {
			service.Record(ctx, object)
		}},
		NewNoopProcessor(),
	)
	go func() {
		defer close(done)
		service.Discard(ctx, object)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("discard held service lock while deleting processing record from store")
	}
}

func TestStoreRecordDoesNotHoldLockDuringStoreUpsert(t *testing.T) {
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "upsert-lock.txt", VersionID: "v1"}
	done := make(chan struct{})
	var service *Service
	service = NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{onUpsert: func() {
			service.Record(ctx, object)
		}},
		NewNoopProcessor(),
	)
	go func() {
		defer close(done)
		if err := service.storeRecord(ctx, object, StatusSucceeded, "", collectionlist.NewList[ProcessorResult]()); err != nil {
			t.Errorf("store processing record: %v", err)
		}
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("storeRecord held service lock while upserting processing record")
	}
}

func TestListRecordsFiltersDiscardedStoreRecords(t *testing.T) {
	ctx := context.Background()
	object := ObjectRef{Bucket: "docs", Key: "discarded-list.txt", VersionID: "v1"}
	storeRecord := model.ProcessingRecord{Bucket: object.Bucket, Key: object.Key, VersionID: object.VersionID, Mode: ModeAsyncPermissive, Status: StatusSucceeded}
	service := NewServiceWithStore(
		slog.New(slog.DiscardHandler),
		Config{Enabled: true, Mode: ModeAsyncPermissive, Timeout: time.Second},
		hookRecordStore{records: collectionlist.NewList(storeRecord)},
		NewNoopProcessor(),
	)
	service.Discard(ctx, object)
	records, err := service.ListRecords(ctx, StatusSucceeded, 10)
	if err != nil {
		t.Fatalf("list records: %v", err)
	}
	if records.Len() != 0 {
		t.Fatalf("records len = %d, want tombstoned store records filtered", records.Len())
	}
}
