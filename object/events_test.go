package object_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/arcgolabs/eventx"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/object"
)

func TestPutObjectPublishesUpdatedEventAfterCommit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	objects, recorder := newEventTestService(t)
	mustCreateEventBucket(ctx, t, objects)

	meta, err := objects.PutObject(ctx, "events", "object.txt", strings.NewReader("event payload"), object.PutOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	stat, err := objects.StatObject(ctx, "events", "object.txt")
	if err != nil {
		t.Fatalf("stat object: %v", err)
	}

	event := recorder.requireSingleEvent(t, "object.updated")
	assertEventPayloadMatchesMeta(t, event, stat)
	if payloadStringValue(event.Payload, "hash") != meta.Hash {
		t.Fatalf("event hash = %q, want put hash %q", payloadStringValue(event.Payload, "hash"), meta.Hash)
	}
}

func TestDeleteObjectPublishesDeletedEventAfterCommit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	objects, recorder := newEventTestService(t)
	mustCreateEventBucket(ctx, t, objects)
	meta := mustPutEventObject(ctx, t, objects, "object.txt", "delete event payload")
	recorder.reset()

	deleted, err := objects.DeleteObject(ctx, "events", "object.txt")
	if err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if _, err := objects.StatObject(ctx, "events", "object.txt"); err == nil {
		t.Fatal("stat deleted object: expected error")
	}

	event := recorder.requireSingleEvent(t, "object.deleted")
	assertEventPayloadMatchesMeta(t, event, deleted)
	if payloadStringValue(event.Payload, "hash") != meta.Hash {
		t.Fatalf("event hash = %q, want deleted hash %q", payloadStringValue(event.Payload, "hash"), meta.Hash)
	}
}

func TestPutObjectFailureDoesNotPublishEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	objects, recorder := newEventTestService(t)

	if _, err := objects.PutObject(ctx, "missing", "object.txt", strings.NewReader("payload"), object.PutOptions{}); err == nil {
		t.Fatal("put object: expected missing bucket error")
	}
	recorder.requireNoEvents(t)
}

func TestDeleteObjectFailureDoesNotPublishEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	objects, recorder := newEventTestService(t)
	mustCreateEventBucket(ctx, t, objects)

	if _, err := objects.DeleteObject(ctx, "events", "missing.txt"); err == nil {
		t.Fatal("delete object: expected missing object error")
	}
	recorder.requireNoEvents(t)
}

func TestOverwritePublishesCommittedUpdatedEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	objects, recorder := newEventTestService(t)
	mustCreateEventBucket(ctx, t, objects)
	original := mustPutEventObject(ctx, t, objects, "object.txt", "original payload")
	recorder.reset()

	replacement, err := objects.PutObject(ctx, "events", "object.txt", strings.NewReader("replacement payload"), object.PutOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("overwrite object: %v", err)
	}
	stat, err := objects.StatObject(ctx, "events", "object.txt")
	if err != nil {
		t.Fatalf("stat overwritten object: %v", err)
	}
	if stat.Hash == original.Hash {
		t.Fatalf("overwritten hash = %q, want different from original", stat.Hash)
	}

	event := recorder.requireSingleEvent(t, "object.updated")
	assertEventPayloadMatchesMeta(t, event, stat)
	if payloadStringValue(event.Payload, "hash") != replacement.Hash {
		t.Fatalf("event hash = %q, want replacement hash %q", payloadStringValue(event.Payload, "hash"), replacement.Hash)
	}
}

type eventRecorder struct {
	events []object.ObjectEvent
}

func newEventTestService(t *testing.T) (*object.Service, *eventRecorder) {
	t.Helper()

	storage, err := store.NewStore(t.TempDir(), metadata.NewInMemoryMetadata(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	bus := eventx.New()
	t.Cleanup(func() {
		if closeErr := bus.Close(); closeErr != nil {
			t.Errorf("close event bus: %v", closeErr)
		}
	})
	recorder := &eventRecorder{}
	unsubscribe, err := eventx.Subscribe(bus, func(_ context.Context, event object.ObjectEvent) error {
		recorder.events = append(recorder.events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("subscribe object events: %v", err)
	}
	t.Cleanup(unsubscribe)

	objects := object.NewService(storage, nil, bus, slog.New(slog.DiscardHandler), object.Config{})
	return objects, recorder
}

func mustCreateEventBucket(ctx context.Context, t *testing.T, objects *object.Service) {
	t.Helper()

	if err := objects.CreateBucket(ctx, "events"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
}

func mustPutEventObject(
	ctx context.Context,
	t *testing.T,
	objects *object.Service,
	key string,
	content string,
) object.ObjectMeta {
	t.Helper()

	meta, err := objects.PutObject(ctx, "events", key, strings.NewReader(content), object.PutOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	return meta
}

func (recorder *eventRecorder) reset() {
	recorder.events = nil
}

func (recorder *eventRecorder) requireNoEvents(t *testing.T) {
	t.Helper()

	if len(recorder.events) != 0 {
		t.Fatalf("events = %+v, want none", recorder.events)
	}
}

func (recorder *eventRecorder) requireSingleEvent(t *testing.T, eventName string) object.ObjectEvent {
	t.Helper()

	if len(recorder.events) != 1 {
		t.Fatalf("event count = %d, want 1: %+v", len(recorder.events), recorder.events)
	}
	event := recorder.events[0]
	if event.Event != eventName {
		t.Fatalf("event name = %q, want %q", event.Event, eventName)
	}
	return event
}

func assertEventPayloadMatchesMeta(t *testing.T, event object.ObjectEvent, meta object.ObjectMeta) {
	t.Helper()

	required := map[string]string{
		"bucket": meta.Bucket,
		"key":    meta.Key,
		"hash":   meta.Hash,
		"etag":   meta.ETag,
	}
	for key, want := range required {
		if got := payloadStringValue(event.Payload, key); got != want {
			t.Fatalf("payload[%s] = %q, want %q", key, got, want)
		}
	}
}

func payloadStringValue(payload map[string]any, key string) string {
	return fmt.Sprint(payload[key])
}
