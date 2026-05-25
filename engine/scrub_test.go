package engine_test

import (
	"bytes"
	"context"
	"testing"
)

func TestScrubObjectVerifiesHealthyObject(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte("scrub should verify shard checksums and object checksum")
	meta, err := e.PutObject(ctx, "test-bucket", "scrub-key.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	result, err := e.ScrubObject(ctx, "test-bucket", "scrub-key.txt")
	if err != nil {
		t.Fatalf("ScrubObject: %v", err)
	}
	if !result.Healthy {
		t.Fatalf("Healthy = false, result = %+v", result)
	}
	if !result.ObjectVerified {
		t.Fatal("ObjectVerified = false, want true")
	}
	if result.ExpectedHash != meta.Hash {
		t.Fatalf("ExpectedHash = %s, want %s", result.ExpectedHash, meta.Hash)
	}
	if result.ActualHash != meta.Hash {
		t.Fatalf("ActualHash = %s, want %s", result.ActualHash, meta.Hash)
	}
}

func TestScrubObjectReportsCorruptedShard(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte("scrub should report corrupted shards without decoding")
	meta, err := e.PutObject(ctx, "test-bucket", "scrub-corrupt-key.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	writeErr := e.WriteLocalShard(ctx, meta.ShardDir, meta.Hash, 0, []byte("corrupted-shard"))
	if writeErr != nil {
		t.Fatalf("corrupt local shard: %v", writeErr)
	}

	result, err := e.ScrubObject(ctx, "test-bucket", "scrub-corrupt-key.txt")
	if err != nil {
		t.Fatalf("ScrubObject: %v", err)
	}
	if result.Healthy {
		t.Fatalf("Healthy = true, result = %+v", result)
	}
	if result.ObjectVerified {
		t.Fatal("ObjectVerified = true, want false")
	}
	if result.Health.Corrupted != 1 {
		t.Fatalf("Corrupted = %d, want 1", result.Health.Corrupted)
	}
}
