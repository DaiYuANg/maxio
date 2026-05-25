package engine_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/model"
)

func TestPutAndGetObjectWithThreeRemoteStorageNodePlacement(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t)

	remoteNodes := []*inMemoryStorageNode{
		newInMemoryStorageNode("node-a", "127.0.0.1:7001"),
		newInMemoryStorageNode("node-b", "127.0.0.1:7002"),
		newInMemoryStorageNode("node-c", "127.0.0.1:7003"),
	}
	if err := registerDistributedPlacementNodes(t, eng, remoteNodes...); err != nil {
		t.Fatal(err)
	}

	content := []byte("three remote storage node distributed placement read/write payload")
	meta, err := eng.PutObject(ctx, "test-bucket", "three-node-placement.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	placementCounts := countDistinctPlacementNodes(meta.ShardPlacements)
	if len(placementCounts) < 3 {
		t.Fatalf("distinct placement nodes = %d, want at least 3: %#v", len(placementCounts), placementCounts)
	}
	assertRemoteNodesOwnPlacedShards(ctx, t, eng, meta, remoteNodes)

	reader, gotInfo, err := eng.GetObject(ctx, "test-bucket", "three-node-placement.txt")
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
	if gotInfo.Hash != meta.Hash {
		t.Fatalf("read hash = %q, want %q", gotInfo.Hash, meta.Hash)
	}

	assertStorageNodeInfosReflectPlacementOwnership(t, eng.StorageNodeInfos(), placementCounts)
}

func countDistinctPlacementNodes(placements []model.ShardPlacement) map[string]int {
	counts := make(map[string]int)
	for _, placement := range placements {
		counts[placementNodeID(placement)]++
	}
	return counts
}

func placementNodeID(placement model.ShardPlacement) string {
	if placement.NodeID == "" {
		return engine.DefaultLocalNodeID
	}
	return placement.NodeID
}

func assertRemoteNodesOwnPlacedShards(
	ctx context.Context,
	t *testing.T,
	eng *engine.Engine,
	meta engine.ObjectInfo,
	remoteNodes []*inMemoryStorageNode,
) {
	t.Helper()

	remoteByID := remoteNodesByID(remoteNodes)
	remoteShardCounts := make(map[string]int, len(remoteNodes))
	for _, placement := range meta.ShardPlacements {
		nodeID := placementNodeID(placement)
		if nodeID == engine.DefaultLocalNodeID {
			assertLocalPlacedShardExists(ctx, t, eng, meta, placement.Index)
			continue
		}
		assertRemotePlacedShardExists(ctx, t, remoteByID, meta, placement)
		remoteShardCounts[nodeID]++
	}

	assertEveryRemoteNodeOwnsShard(t, remoteNodes, remoteShardCounts)
}

func remoteNodesByID(nodes []*inMemoryStorageNode) map[string]*inMemoryStorageNode {
	remoteByID := make(map[string]*inMemoryStorageNode, len(nodes))
	for _, node := range nodes {
		remoteByID[node.id] = node
	}
	return remoteByID
}

func assertLocalPlacedShardExists(
	ctx context.Context,
	t *testing.T,
	eng *engine.Engine,
	meta engine.ObjectInfo,
	index int,
) {
	t.Helper()

	if !eng.LocalShardExists(ctx, meta.ShardDir, meta.Hash, index) {
		t.Fatalf("expected local shard %d to exist", index)
	}
}

func assertRemotePlacedShardExists(
	ctx context.Context,
	t *testing.T,
	remoteByID map[string]*inMemoryStorageNode,
	meta engine.ObjectInfo,
	placement model.ShardPlacement,
) {
	t.Helper()

	nodeID := placementNodeID(placement)
	node := remoteByID[nodeID]
	if node == nil {
		t.Fatalf("placement references unregistered remote node %q", nodeID)
	}
	if !node.ShardExists(ctx, meta.ShardDir, meta.Hash, placement.Index) {
		t.Fatalf("expected shard %d on remote node %q", placement.Index, nodeID)
	}
}

func assertEveryRemoteNodeOwnsShard(t *testing.T, remoteNodes []*inMemoryStorageNode, remoteShardCounts map[string]int) {
	t.Helper()

	for _, node := range remoteNodes {
		if remoteShardCounts[node.id] == 0 {
			t.Fatalf("expected remote node %q to own at least one shard", node.id)
		}
	}
}

func assertStorageNodeInfosReflectPlacementOwnership(
	t *testing.T,
	infos []engine.StorageNodeInfo,
	placementCounts map[string]int,
) {
	t.Helper()

	for nodeID, shardCount := range placementCounts {
		info := findStorageNodeInfo(t, infos, nodeID)
		if info.ObjectCount != 1 {
			t.Fatalf("node %q object_count = %d, want 1", nodeID, info.ObjectCount)
		}
		if info.ShardCount != shardCount {
			t.Fatalf("node %q shard_count = %d, want %d", nodeID, info.ShardCount, shardCount)
		}
		if info.UsedBytes == 0 {
			t.Fatalf("node %q used_bytes = 0, want > 0", nodeID)
		}
	}
}
