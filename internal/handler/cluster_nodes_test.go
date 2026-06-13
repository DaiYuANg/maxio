package handler_test

import (
	"slices"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/control"
	"github.com/lyonbrown4d/maxio/internal/discovery"
	"github.com/lyonbrown4d/maxio/internal/handler"
)

func TestBuildClusterNodeRegistryMergesMemberDiscoveryAndStorage(t *testing.T) {
	t.Parallel()

	nodes := handler.BuildClusterNodeRegistry(control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
			2: "127.0.0.1:63001",
		},
	}, []discovery.Node{
		{ReplicaID: 1, State: "alive", ControlAddress: "127.0.0.1:63000", HTTPAddress: "127.0.0.1:8080"},
		{ReplicaID: 2, State: "suspect", ControlAddress: "127.0.0.1:63001", HTTPAddress: "127.0.0.1:8081"},
		{ReplicaID: 3, State: "alive", ControlAddress: "127.0.0.1:63002", HTTPAddress: "127.0.0.1:8082"},
	}, []handler.StorageNodeInfo{
		{ID: "node-1", Address: "127.0.0.1:8080", Local: true},
		{ID: "node-2", Address: "127.0.0.1:8081", ObjectCount: 1, ShardCount: 4, UsedBytes: 128},
	})

	if len(nodes) != 3 {
		t.Fatalf("nodes = %+v, want 3", nodes)
	}
	assertClusterNode(t, nodes[0], 1, handler.ClusterNodeOnline)
	assertClusterNode(t, nodes[1], 2, handler.ClusterNodeSuspect)
	assertClusterNode(t, nodes[2], 3, handler.ClusterNodeDiscovered)
	assertClusterStorageState(t, nodes[0], handler.ClusterStorageStateActive)
	assertClusterStorageState(t, nodes[1], handler.ClusterStorageStateActive)
	assertClusterStorageState(t, nodes[2], handler.ClusterStorageStateUnregistered)
	if nodes[1].ObjectCount != 1 || nodes[1].ShardCount != 4 {
		t.Fatalf("node 2 ownership = %d/%d, want 1/4", nodes[1].ObjectCount, nodes[1].ShardCount)
	}
	if nodes[1].UsedBytes != 128 {
		t.Fatalf("node 2 used_bytes = %d, want 128", nodes[1].UsedBytes)
	}
	if !slices.Contains(nodes[2].Issues, "not_in_control_membership") {
		t.Fatalf("node 3 issues = %+v, want not_in_control_membership", nodes[2].Issues)
	}
}

func TestBuildClusterNodeRegistryReportsOfflineMissingStorage(t *testing.T) {
	t.Parallel()

	nodes := handler.BuildClusterNodeRegistry(control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
			2: "127.0.0.1:63001",
		},
	}, []discovery.Node{
		{ReplicaID: 1, State: "alive", ControlAddress: "127.0.0.1:63000"},
	}, []handler.StorageNodeInfo{
		{ID: "node-1", Address: "127.0.0.1:8080", Local: true},
	})

	assertClusterNode(t, nodes[1], 2, handler.ClusterNodeOffline)
	assertClusterStorageState(t, nodes[1], handler.ClusterStorageStateUnregistered)
	if !slices.Contains(nodes[1].Issues, "not_discovered") {
		t.Fatalf("node 2 issues = %+v, want not_discovered", nodes[1].Issues)
	}
}

func TestBuildClusterNodeRegistryReportsDrainingStorageNode(t *testing.T) {
	t.Parallel()

	nodes := handler.BuildClusterNodeRegistry(control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
		},
	}, []discovery.Node{
		{ReplicaID: 1, State: "alive", ControlAddress: "127.0.0.1:63000"},
	}, []handler.StorageNodeInfo{
		{ID: "node-1", Address: "127.0.0.1:8080", Local: true, Drained: true, ObjectCount: 1, ShardCount: 3, UsedBytes: 64},
	})

	assertClusterNode(t, nodes[0], 1, handler.ClusterNodeDraining)
	assertClusterStorageState(t, nodes[0], handler.ClusterStorageStateDrained)
	if !slices.Contains(nodes[0].Issues, "drained_storage_has_ownership") {
		t.Fatalf("node issues = %+v, want drained_storage_has_ownership", nodes[0].Issues)
	}
}

func TestBuildClusterNodeRegistryReportsRemovedNodeReappearance(t *testing.T) {
	t.Parallel()

	nodes := handler.BuildClusterNodeRegistry(control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
		},
		Removed: []uint64{2},
	}, []discovery.Node{
		{ReplicaID: 2, State: "alive", ControlAddress: "127.0.0.1:63001", HTTPAddress: "127.0.0.1:8081"},
	}, []handler.StorageNodeInfo{
		{ID: "node-2", Address: "127.0.0.1:8081"},
	})

	assertClusterNode(t, nodes[1], 2, handler.ClusterNodeDiscovered)
	if !nodes[1].Removed {
		t.Fatalf("removed = false, want true: %+v", nodes[1])
	}
	if !slices.Contains(nodes[1].Issues, "removed_node_reappeared") {
		t.Fatalf("node issues = %+v, want removed_node_reappeared", nodes[1].Issues)
	}
}

func assertClusterNode(t *testing.T, node handler.ClusterNodeInfo, replicaID uint64, status string) {
	t.Helper()
	if node.ReplicaID != replicaID {
		t.Fatalf("replica id = %d, want %d: %+v", node.ReplicaID, replicaID, node)
	}
	if node.Status != status {
		t.Fatalf("status = %q, want %q: %+v", node.Status, status, node)
	}
}

func assertClusterStorageState(t *testing.T, node handler.ClusterNodeInfo, state string) {
	t.Helper()
	if node.StorageState != state {
		t.Fatalf("storage_state = %q, want %q: %+v", node.StorageState, state, node)
	}
}
