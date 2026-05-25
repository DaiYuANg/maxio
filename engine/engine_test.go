package engine_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/model"
	"github.com/spf13/afero"
)

const defaultTotalChunks = engine.DefaultDataChunks + engine.DefaultParityChunks

func newTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	fs := afero.NewMemMapFs()
	e, err := engine.NewEngine("/test", engine.DefaultDataChunks, engine.DefaultParityChunks, fs)
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	return e
}

func assertDefaultLocalPlacements(t *testing.T, placements []model.ShardPlacement) {
	t.Helper()
	if len(placements) != defaultTotalChunks {
		t.Fatalf("ShardPlacements = %d, want %d", len(placements), defaultTotalChunks)
	}
	for index := range placements {
		placement := placements[index]
		if placement.Index != index {
			t.Errorf("ShardPlacements[%d].Index = %d, want %d", index, placement.Index, index)
		}
		if placement.NodeID != engine.DefaultLocalNodeID {
			t.Errorf("ShardPlacements[%d].NodeID = %q, want %q", index, placement.NodeID, engine.DefaultLocalNodeID)
		}
	}
}

func assertShardChecksums(t *testing.T, checksums []string) {
	t.Helper()
	if len(checksums) != defaultTotalChunks {
		t.Fatalf("ShardChecksums = %d, want %d", len(checksums), defaultTotalChunks)
	}
	for index, checksum := range checksums {
		if checksum == "" {
			t.Fatalf("ShardChecksums[%d] is empty", index)
		}
	}
}

func TestNewEngine(t *testing.T) {
	fs := afero.NewMemMapFs()
	storage, err := engine.NewEngine("/test", engine.DefaultDataChunks, engine.DefaultParityChunks, fs)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}
	if storage == nil {
		t.Fatal("engine is nil")
	}
}

func TestPutAndGetObject(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte("hello world, this is a test object for erasure coding")
	meta, err := e.PutObject(ctx, "test-bucket", "test-key.txt", strings.NewReader(string(content)), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if meta.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", meta.Size, len(content))
	}
	if meta.ETag == "" {
		t.Error("ETag is empty")
	}
	assertDefaultLocalPlacements(t, meta.ShardPlacements)
	assertShardChecksums(t, meta.ShardChecksums)

	reader, objInfo, err := e.GetObject(ctx, "test-bucket", "test-key.txt")
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
		t.Errorf("data = %s, want %s", data, content)
	}
	if objInfo.ETag != meta.ETag {
		t.Errorf("ETag = %s, want %s", objInfo.ETag, meta.ETag)
	}
}

func TestGetObjectAfterEngineRestart(t *testing.T) {
	ctx := context.Background()
	fs := afero.NewMemMapFs()
	first, err := engine.NewEngine("/test", engine.DefaultDataChunks, engine.DefaultParityChunks, fs)
	if err != nil {
		t.Fatalf("create first engine: %v", err)
	}
	content := []byte("restart should not lose layout lookup")
	meta, err := first.PutObject(ctx, "test-bucket", "restart-key.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	restarted, err := engine.NewEngine("/test", engine.DefaultDataChunks, engine.DefaultParityChunks, fs)
	if err != nil {
		t.Fatalf("create restarted engine: %v", err)
	}
	reader, objInfo, err := restarted.GetObject(ctx, "test-bucket", "restart-key.txt")
	if err != nil {
		t.Fatalf("GetObject after restart: %v", err)
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
		t.Errorf("data = %q, want %q", data, content)
	}
	if objInfo.Hash != meta.Hash {
		t.Errorf("Hash = %s, want %s", objInfo.Hash, meta.Hash)
	}
	if objInfo.ContentType != meta.ContentType {
		t.Errorf("ContentType = %s, want %s", objInfo.ContentType, meta.ContentType)
	}
	assertDefaultLocalPlacements(t, objInfo.ShardPlacements)
	assertShardChecksums(t, objInfo.ShardChecksums)
}

func TestGetObjectPreservesTrailingZeroBytes(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte{'b', 'i', 'n', 0, 0}
	_, err := e.PutObject(ctx, "test-bucket", "binary-key.bin", bytes.NewReader(content), "application/octet-stream")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	reader, _, err := e.GetObject(ctx, "test-bucket", "binary-key.bin")
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
		t.Errorf("data = %v, want %v", data, content)
	}
}

func TestDeleteObject(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte("delete me")
	_, err := e.PutObject(ctx, "test-bucket", "delete-key.txt", strings.NewReader(string(content)), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	err = e.DeleteObject(ctx, "test-bucket", "delete-key.txt")
	if err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}

	_, _, err = e.GetObject(ctx, "test-bucket", "delete-key.txt")
	if !errors.Is(err, engine.ErrObjectNotFound) {
		t.Errorf("GetObject after delete = %v, want ErrObjectNotFound", err)
	}
}

func TestHealthCheck(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte("health check test")
	_, err := e.PutObject(ctx, "test-bucket", "health-key.txt", strings.NewReader(string(content)), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	health, err := e.CheckHealth(ctx, "test-bucket", "health-key.txt")
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if health.TotalChunks != engine.DefaultDataChunks+engine.DefaultParityChunks {
		t.Errorf("TotalChunks = %d, want %d", health.TotalChunks, engine.DefaultDataChunks+engine.DefaultParityChunks)
	}
	if health.Available != health.TotalChunks {
		t.Errorf("Available = %d, want %d", health.Available, health.TotalChunks)
	}
	if health.Corrupted != 0 {
		t.Errorf("Corrupted = %d, want 0", health.Corrupted)
	}
	if !health.Recoverable {
		t.Error("Recoverable should be true when all shards are present")
	}
}

func TestHealthDetectsCorruptedShardAndReadRecovers(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	content := []byte("corrupted shard should be detected and recovered")
	meta, err := e.PutObject(ctx, "test-bucket", "corrupt-key.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	if corruptErr := e.WriteLocalShard(ctx, meta.ShardDir, meta.Hash, 0, []byte("corrupted-shard")); corruptErr != nil {
		t.Fatalf("corrupt local shard: %v", corruptErr)
	}

	health, err := e.CheckHealth(ctx, "test-bucket", "corrupt-key.txt")
	if err != nil {
		t.Fatalf("CheckHealth: %v", err)
	}
	if health.Corrupted != 1 {
		t.Fatalf("Corrupted = %d, want 1", health.Corrupted)
	}
	if !health.Recoverable {
		t.Fatal("Recoverable should be true with one corrupted shard")
	}

	reader, _, err := e.GetObject(ctx, "test-bucket", "corrupt-key.txt")
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

func TestGetObjectFailsWhenTooManyShardsAreCorrupted(t *testing.T) {
	ctx := context.Background()
	e := newTestEngine(t)

	meta, err := e.PutObject(ctx, "test-bucket", "unrecoverable-key.txt", strings.NewReader("unrecoverable shard damage"), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	corruptCount := engine.DefaultParityChunks + 1
	for index := range corruptCount {
		if corruptErr := e.WriteLocalShard(ctx, meta.ShardDir, meta.Hash, index, []byte("corrupted-shard")); corruptErr != nil {
			t.Fatalf("corrupt local shard %d: %v", index, corruptErr)
		}
	}

	_, _, err = e.GetObject(ctx, "test-bucket", "unrecoverable-key.txt")
	if !errors.Is(err, engine.ErrShardRecoveryFailed) {
		t.Fatalf("GetObject error = %v, want ErrShardRecoveryFailed", err)
	}
}
