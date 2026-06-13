package engine_test

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/model"
)

const remoteRepairObjectKey = "remote-repair-key.txt"

func TestRepairObjectUsesRemoteHealthyShards(t *testing.T) {
	ctx := context.Background()
	node1, node2, cleanup := newRemoteHTTPRepairEngines(t)
	defer cleanup()

	content := []byte("remote healthy shards should repair corrupted local shards")
	meta := putRemoteRepairObject(ctx, t, node1, node2, content)
	localIndex := corruptLocalPlacement(ctx, t, node1, meta)
	result := repairRemoteObject(ctx, t, node1)

	assertRemoteRepairResult(t, result, localIndex)
	assertRemoteRepairObjectReadable(ctx, t, node1, content)
}

func newRemoteHTTPRepairEngines(t *testing.T) (*engine.Engine, *engine.Engine, func()) {
	t.Helper()
	node1 := newTestEngine(t)
	node2 := newTestEngine(t)
	server := httptest.NewServer(storageShardHandler(node2))
	if err := node1.SyncStorageNodesFromControl(1, map[uint64]string{
		1: "127.0.0.1:9001",
		2: server.URL,
	}); err != nil {
		server.Close()
		t.Fatalf("sync storage nodes: %v", err)
	}
	return node1, node2, server.Close
}

func putRemoteRepairObject(
	ctx context.Context,
	t *testing.T,
	node1 *engine.Engine,
	node2 *engine.Engine,
	content []byte,
) engine.ObjectInfo {
	t.Helper()
	meta, err := node1.PutObject(ctx, "test-bucket", remoteRepairObjectKey, bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	assertRemoteHTTPShardsStored(ctx, t, node2, meta)
	return meta
}

func corruptLocalPlacement(ctx context.Context, t *testing.T, node *engine.Engine, meta engine.ObjectInfo) int {
	t.Helper()
	localIndex := firstPlacementIndex(t, meta.ShardPlacements, "node-1")
	err := node.WriteLocalShard(ctx, meta.ShardDir, meta.Hash, localIndex, []byte("corrupted local shard"))
	if err != nil {
		t.Fatalf("corrupt local shard: %v", err)
	}
	return localIndex
}

func repairRemoteObject(ctx context.Context, t *testing.T, node *engine.Engine) engine.RepairResult {
	t.Helper()
	result, err := node.RepairObject(ctx, "test-bucket", remoteRepairObjectKey)
	if err != nil {
		t.Fatalf("RepairObject: %v", err)
	}
	return result
}

func assertRemoteRepairResult(t *testing.T, result engine.RepairResult, localIndex int) {
	t.Helper()
	if result.HealthBefore.Corrupted == 0 {
		t.Fatal("expected repair to detect corrupted local shard")
	}
	if result.HealthAfter.Missing != 0 || result.HealthAfter.Corrupted != 0 {
		t.Fatalf("health after repair = %+v, want no missing or corrupted shards", result.HealthAfter)
	}
	if !slices.Contains(result.Repaired, localIndex) {
		t.Fatalf("repaired shards = %v, want index %d", result.Repaired, localIndex)
	}
}

func assertRemoteRepairObjectReadable(ctx context.Context, t *testing.T, node *engine.Engine, content []byte) {
	t.Helper()
	reader, _, err := node.GetObject(ctx, "test-bucket", remoteRepairObjectKey)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close reader: %v", closeErr)
		}
	}()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read object data: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("data = %q, want %q", got, content)
	}
}

func firstPlacementIndex(t *testing.T, placements []model.ShardPlacement, nodeID string) int {
	t.Helper()
	for _, placement := range placements {
		if placement.NodeID == nodeID {
			return placement.Index
		}
	}
	t.Fatalf("expected at least one shard placed on node %q", nodeID)
	return -1
}
