package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/model"
)

func TestDrainStorageNodeExcludesNewPlacements(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t)
	nodeA := newInMemoryStorageNode("node-a", "127.0.0.1:7001")
	nodeB := newInMemoryStorageNode("node-b", "127.0.0.1:7002")
	if err := registerDistributedPlacementNodes(t, eng, nodeA, nodeB); err != nil {
		t.Fatal(err)
	}

	if err := eng.DrainStorageNode(nodeA.id); err != nil {
		t.Fatalf("drain storage node: %v", err)
	}

	meta := putObjectForDrain(ctx, t, eng, "drain-key.txt")
	assertPlacementExcludesNode(t, meta.ShardPlacements, nodeA.id)
	assertPlacementIncludesNode(t, meta.ShardPlacements, engine.DefaultLocalNodeID)
	assertPlacementIncludesNode(t, meta.ShardPlacements, nodeB.id)
}

func TestResumeStorageNodeRestoresNewPlacements(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t)
	nodeA := newInMemoryStorageNode("node-a", "127.0.0.1:7001")
	nodeB := newInMemoryStorageNode("node-b", "127.0.0.1:7002")
	if err := registerDistributedPlacementNodes(t, eng, nodeA, nodeB); err != nil {
		t.Fatal(err)
	}
	if err := eng.DrainStorageNode(nodeA.id); err != nil {
		t.Fatalf("drain storage node: %v", err)
	}
	if err := eng.ResumeStorageNode(nodeA.id); err != nil {
		t.Fatalf("resume storage node: %v", err)
	}

	meta := putObjectForDrain(ctx, t, eng, "resume-key.txt")
	assertPlacementIncludesNode(t, meta.ShardPlacements, nodeA.id)
}

func TestStorageNodeInfosReportsDrainState(t *testing.T) {
	eng := newTestEngine(t)
	nodeA := newInMemoryStorageNode("node-a", "127.0.0.1:7001")
	if err := registerDistributedPlacementNodes(t, eng, nodeA); err != nil {
		t.Fatal(err)
	}
	if err := eng.DrainStorageNode(nodeA.id); err != nil {
		t.Fatalf("drain storage node: %v", err)
	}

	info := findStorageNodeInfo(t, eng.StorageNodeInfos(), nodeA.id)
	if !info.Drained {
		t.Fatalf("node %q is not reported as drained", nodeA.id)
	}
}

func TestRebalanceObjectFromNodeMovesDrainedPlacements(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t)
	nodeA := newInMemoryStorageNode("node-a", "127.0.0.1:7001")
	nodeB := newInMemoryStorageNode("node-b", "127.0.0.1:7002")
	if err := registerDistributedPlacementNodes(t, eng, nodeA, nodeB); err != nil {
		t.Fatal(err)
	}
	meta := putObjectForDrain(ctx, t, eng, "rebalance-key.txt")
	assertPlacementIncludesNode(t, meta.ShardPlacements, nodeA.id)
	if err := eng.DrainStorageNode(nodeA.id); err != nil {
		t.Fatalf("drain storage node: %v", err)
	}

	result, err := eng.RebalanceObjectFromNode(ctx, "test-bucket", "rebalance-key.txt", nodeA.id)
	if err != nil {
		t.Fatalf("rebalance object: %v", err)
	}
	if len(result.Moved) == 0 {
		t.Fatal("expected moved shards")
	}
	for _, move := range result.Moved {
		if nodeA.ShardExists(ctx, result.Object.ShardDir, result.Object.Hash, move.Index) {
			t.Fatalf("expected old shard %d deleted from drained node", move.Index)
		}
	}
	assertPlacementExcludesNode(t, result.Object.ShardPlacements, nodeA.id)
	assertPlacementIncludesNode(t, result.Object.ShardPlacements, nodeB.id)
}

func putObjectForDrain(ctx context.Context, t *testing.T, eng *engine.Engine, key string) engine.ObjectInfo {
	t.Helper()
	meta, err := eng.PutObject(ctx, "test-bucket", key, strings.NewReader("drain placement payload"), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	return meta
}

func findStorageNodeInfo(t *testing.T, infos []engine.StorageNodeInfo, nodeID string) engine.StorageNodeInfo {
	t.Helper()
	for _, info := range infos {
		if info.ID == nodeID {
			return info
		}
	}
	t.Fatalf("storage node info for %q not found", nodeID)
	return engine.StorageNodeInfo{}
}

func assertPlacementExcludesNode(t *testing.T, placements []model.ShardPlacement, nodeID string) {
	t.Helper()
	for _, placement := range placements {
		if placement.NodeID == nodeID {
			t.Fatalf("placement includes drained node %q", nodeID)
		}
	}
}

func assertPlacementIncludesNode(t *testing.T, placements []model.ShardPlacement, nodeID string) {
	t.Helper()
	for _, placement := range placements {
		if placement.NodeID == nodeID {
			return
		}
	}
	t.Fatalf("placement does not include node %q", nodeID)
}
