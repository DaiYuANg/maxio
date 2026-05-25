package engine_test

import (
	"context"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
)

type testStorageNode struct {
	id      string
	address string
}

func (t *testStorageNode) ID() string      { return t.id }
func (t *testStorageNode) Address() string { return t.address }
func (*testStorageNode) WriteShard(_ context.Context, _, _ string, _ int, _ []byte) error {
	return nil
}
func (*testStorageNode) ReadShard(_ context.Context, _, _ string, _ int) ([]byte, error) {
	return nil, nil
}
func (*testStorageNode) ShardExists(_ context.Context, _, _ string, _ int) bool {
	return false
}
func (*testStorageNode) DeleteShard(_ context.Context, _, _ string, _ int) error {
	return nil
}

func TestRoundRobinPlacementAcrossRegisteredNodes(t *testing.T) {
	e := newTestEngine(t)

	if err := e.RegisterStorageNode(&testStorageNode{id: "node-a", address: "127.0.0.1:7001"}); err != nil {
		t.Fatalf("register node-a: %v", err)
	}
	if err := e.RegisterStorageNode(&testStorageNode{id: "node-b", address: "127.0.0.1:7002"}); err != nil {
		t.Fatalf("register node-b: %v", err)
	}

	placements, err := e.PlanShardPlacement(context.Background(), engine.PlacementRequest{
		Bucket:     "bucket",
		Key:        "key",
		Hash:       "hash",
		ShardCount: 6,
	})
	if err != nil {
		t.Fatalf("plan shard placement: %v", err)
	}
	if len(placements) != 6 {
		t.Fatalf("placements len = %d, want %d", len(placements), 6)
	}

	expected := []string{engine.DefaultLocalNodeID, "node-a", "node-b", engine.DefaultLocalNodeID, "node-a", "node-b"}
	for index, placement := range placements {
		if placement.Index != index {
			t.Errorf("placement index = %d, want %d", placement.Index, index)
		}
		if placement.NodeID != expected[index] {
			t.Errorf("placement[%d].node_id = %q, want %q", index, placement.NodeID, expected[index])
		}
	}
}

func TestRegisterStorageNodeRejectsEmptyID(t *testing.T) {
	e := newTestEngine(t)
	if err := e.RegisterStorageNode(&testStorageNode{id: " ", address: "127.0.0.1:7003"}); err == nil {
		t.Fatal("expected register with empty id to fail")
	}
}

func TestSyncStorageNodesFromRaft(t *testing.T) {
	e := newTestEngine(t)

	err := e.SyncStorageNodesFromRaft(1, map[uint64]string{
		1: "127.0.0.1:7001",
		2: "127.0.0.1:7002",
		3: "127.0.0.1:7003",
	})
	if err != nil {
		t.Fatalf("sync storage nodes: %v", err)
	}

	nodes := e.StorageNodes()
	if len(nodes) != 3 {
		t.Fatalf("storage nodes = %d, want %d", len(nodes), 3)
	}

	expected := map[string]bool{
		"raft-1": false,
		"raft-2": false,
		"raft-3": false,
	}
	for _, node := range nodes {
		nodeID := node.ID()
		if _, ok := expected[nodeID]; ok {
			expected[nodeID] = true
		}
	}
	for nodeID, seen := range expected {
		if !seen {
			t.Errorf("node %s not synced", nodeID)
		}
	}
}

func TestSyncStorageNodesFromRaftUpdatesNodeAddresses(t *testing.T) {
	e := newTestEngine(t)
	if err := e.SyncStorageNodesFromRaft(1, map[uint64]string{
		1: "127.0.0.1:7001",
		2: "127.0.0.1:7002",
	}); err != nil {
		t.Fatalf("sync storage nodes: %v", err)
	}
	if err := e.SyncStorageNodesFromRaft(1, map[uint64]string{
		1: "127.0.0.1:7101",
		2: "127.0.0.1:7102",
	}); err != nil {
		t.Fatalf("sync updated storage nodes: %v", err)
	}

	local, err := e.LocalStorageNode()
	if err != nil {
		t.Fatalf("local storage node: %v", err)
	}
	if local.Address() != "127.0.0.1:7101" {
		t.Fatalf("local address = %q, want %q", local.Address(), "127.0.0.1:7101")
	}
	remote, err := e.StorageNode("raft-2")
	if err != nil {
		t.Fatalf("remote storage node: %v", err)
	}
	if remote.Address() != "127.0.0.1:7102" {
		t.Fatalf("remote address = %q, want %q", remote.Address(), "127.0.0.1:7102")
	}
}

func TestStorageNodeInfosReportShardOwnership(t *testing.T) {
	e := newTestEngine(t)

	if err := e.RegisterStorageNode(&testStorageNode{id: "node-a", address: "127.0.0.1:7001"}); err != nil {
		t.Fatalf("register node-a: %v", err)
	}
	if err := e.RegisterStorageNode(&testStorageNode{id: "node-b", address: "127.0.0.1:7002"}); err != nil {
		t.Fatalf("register node-b: %v", err)
	}
	if _, err := e.PutObject(context.Background(), "bucket", "key", strings.NewReader("ownership payload"), "text/plain"); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	local := findStorageNodeInfo(t, e.StorageNodeInfos(), engine.DefaultLocalNodeID)
	if local.ObjectCount != 1 {
		t.Fatalf("local object_count = %d, want 1", local.ObjectCount)
	}
	if local.ShardCount == 0 {
		t.Fatalf("local shard_count = 0, want > 0")
	}
	if local.UsedBytes == 0 {
		t.Fatalf("local used_bytes = 0, want > 0")
	}
	nodeA := findStorageNodeInfo(t, e.StorageNodeInfos(), "node-a")
	if nodeA.ObjectCount != 1 {
		t.Fatalf("node-a object_count = %d, want 1", nodeA.ObjectCount)
	}
	if nodeA.ShardCount == 0 {
		t.Fatalf("node-a shard_count = 0, want > 0")
	}
	if nodeA.UsedBytes == 0 {
		t.Fatalf("node-a used_bytes = 0, want > 0")
	}
}
