package engine_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
)

func TestRepairObjectRestoresMissingLocalShard(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte("missing local shard should be rebuilt by repair")
	meta, err := e.PutObject(ctx, "test-bucket", "missing-shard.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	const missingIndex = 0
	deleteMissingLocalShard(ctx, t, e, meta, missingIndex)
	assertMissingShardHealth(ctx, t, e)

	result, err := e.RepairObject(ctx, "test-bucket", "missing-shard.txt")
	if err != nil {
		t.Fatalf("RepairObject: %v", err)
	}
	assertMissingShardRepairResult(ctx, t, e, meta, result, missingIndex)
	assertMissingShardObjectReadable(ctx, t, e, content)
}

func TestRepairObjectRestoresCorruptedLocalShard(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte("corrupted local shard should be rebuilt by repair")
	meta, err := e.PutObject(ctx, "test-bucket", "corrupted-shard.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	const corruptedIndex = 0
	corruptLocalShard(ctx, t, e, meta, corruptedIndex)
	assertCorruptedShardHealth(ctx, t, e)

	result, err := e.RepairObject(ctx, "test-bucket", "corrupted-shard.txt")
	if err != nil {
		t.Fatalf("RepairObject: %v", err)
	}
	assertCorruptedShardRepairResult(t, result, corruptedIndex)
	assertCorruptedShardObjectReadable(ctx, t, e, content)
}

func TestRepairObjectReportsUnrecoverableMissingShards(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	meta, err := e.PutObject(ctx, "test-bucket", "unrecoverable-missing.txt", bytes.NewReader([]byte("too many missing shards")), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	missingCount := engine.DefaultParityChunks + 1
	deleteLocalShards(ctx, t, e, meta, missingCount)

	result, err := e.RepairObject(ctx, "test-bucket", "unrecoverable-missing.txt")
	if !errors.Is(err, engine.ErrShardRecoveryFailed) {
		t.Fatalf("RepairObject error = %v, want ErrShardRecoveryFailed", err)
	}
	assertUnrecoverableRepairResult(t, result, missingCount)
}

func deleteLocalShards(
	ctx context.Context,
	t *testing.T,
	e *engine.Engine,
	meta engine.ObjectInfo,
	count int,
) {
	t.Helper()
	for index := range count {
		deleteErr := e.DeleteLocalShard(ctx, meta.ShardDir, meta.Hash, index)
		if deleteErr != nil {
			t.Fatalf("delete local shard %d: %v", index, deleteErr)
		}
	}
}

func deleteMissingLocalShard(
	ctx context.Context,
	t *testing.T,
	e *engine.Engine,
	meta engine.ObjectInfo,
	missingIndex int,
) {
	t.Helper()
	deleteErr := e.DeleteLocalShard(ctx, meta.ShardDir, meta.Hash, missingIndex)
	if deleteErr != nil {
		t.Fatalf("delete local shard: %v", deleteErr)
	}
}

func corruptLocalShard(
	ctx context.Context,
	t *testing.T,
	e *engine.Engine,
	meta engine.ObjectInfo,
	corruptedIndex int,
) {
	t.Helper()
	corruptErr := e.WriteLocalShard(ctx, meta.ShardDir, meta.Hash, corruptedIndex, []byte("corrupted shard"))
	if corruptErr != nil {
		t.Fatalf("corrupt local shard: %v", corruptErr)
	}
}

func assertUnrecoverableRepairResult(t *testing.T, result engine.RepairResult, missingCount int) {
	t.Helper()
	if result.HealthBefore.Missing != missingCount {
		t.Fatalf("HealthBefore.Missing = %d, want %d", result.HealthBefore.Missing, missingCount)
	}
	if result.HealthAfter.Missing != missingCount {
		t.Fatalf("HealthAfter.Missing = %d, want %d", result.HealthAfter.Missing, missingCount)
	}
	if result.HealthAfter.Recoverable {
		t.Fatal("HealthAfter.Recoverable = true, want false")
	}
	if len(result.Repaired) != 0 {
		t.Fatalf("Repaired = %v, want empty", result.Repaired)
	}
}

func assertMissingShardHealth(ctx context.Context, t *testing.T, e *engine.Engine) {
	t.Helper()
	health, err := e.CheckHealth(ctx, "test-bucket", "missing-shard.txt")
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if health.Missing != 1 {
		t.Fatalf("Missing = %d, want 1", health.Missing)
	}
	if !health.Recoverable {
		t.Fatal("Recoverable should be true with one missing shard")
	}
}

func assertCorruptedShardHealth(ctx context.Context, t *testing.T, e *engine.Engine) {
	t.Helper()
	health, err := e.CheckHealth(ctx, "test-bucket", "corrupted-shard.txt")
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if health.Corrupted != 1 {
		t.Fatalf("Corrupted = %d, want 1", health.Corrupted)
	}
	if !health.Recoverable {
		t.Fatal("Recoverable should be true with one corrupted shard")
	}
}

func assertMissingShardRepairResult(
	ctx context.Context,
	t *testing.T,
	e *engine.Engine,
	meta engine.ObjectInfo,
	result engine.RepairResult,
	missingIndex int,
) {
	t.Helper()
	if !intSliceContains(result.Repaired, missingIndex) {
		t.Fatalf("repaired shards = %v, want %d", result.Repaired, missingIndex)
	}
	if result.HealthAfter.Missing != 0 || result.HealthAfter.Corrupted != 0 {
		t.Fatalf("HealthAfter = %+v, want no missing or corrupted shards", result.HealthAfter)
	}
	if !e.LocalShardExists(ctx, meta.ShardDir, meta.Hash, missingIndex) {
		t.Fatal("repaired shard does not exist")
	}
}

func assertCorruptedShardRepairResult(t *testing.T, result engine.RepairResult, corruptedIndex int) {
	t.Helper()
	if !intSliceContains(result.Repaired, corruptedIndex) {
		t.Fatalf("repaired shards = %v, want %d", result.Repaired, corruptedIndex)
	}
	if result.HealthAfter.Missing != 0 || result.HealthAfter.Corrupted != 0 {
		t.Fatalf("HealthAfter = %+v, want no missing or corrupted shards", result.HealthAfter)
	}
}

func assertMissingShardObjectReadable(ctx context.Context, t *testing.T, e *engine.Engine, content []byte) {
	t.Helper()
	reader, _, err := e.GetObject(ctx, "test-bucket", "missing-shard.txt")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close reader: %v", closeErr)
		}
	}()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read object data: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("data = %q, want %q", data, content)
	}
}

func assertCorruptedShardObjectReadable(ctx context.Context, t *testing.T, e *engine.Engine, content []byte) {
	t.Helper()
	reader, _, err := e.GetObject(ctx, "test-bucket", "corrupted-shard.txt")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close reader: %v", closeErr)
		}
	}()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read object data: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("data = %q, want %q", data, content)
	}
}

func intSliceContains(values []int, expected int) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
