package handler

import (
	"errors"
	"fmt"
	"testing"
)

func TestDecommissionBlockedResponseIncludesOwnershipStats(t *testing.T) {
	err := fmt.Errorf("blocked: %w", &clusterDecommissionBlockedError{
		replicaID: 2,
		nodeID:    "node-2",
		stats: nodePlacementStats{
			objects:   3,
			shards:    7,
			usedBytes: 2048,
		},
	})

	response, ok := decommissionBlockedResponse(err)

	if !ok {
		t.Fatal("expected decommission blocked response")
	}
	if response.ReplicaID != 2 {
		t.Fatalf("replica_id = %d, want 2", response.ReplicaID)
	}
	if response.NodeID != "node-2" {
		t.Fatalf("node_id = %q, want node-2", response.NodeID)
	}
	if response.Objects != 3 {
		t.Fatalf("objects = %d, want 3", response.Objects)
	}
	if response.Shards != 7 {
		t.Fatalf("shards = %d, want 7", response.Shards)
	}
	if response.UsedBytes != 2048 {
		t.Fatalf("used_bytes = %d, want 2048", response.UsedBytes)
	}
	if response.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", response.Status)
	}
	if response.Error == "" {
		t.Fatal("expected response error message")
	}
}

func TestClusterDecommissionBlockedErrorWrapsSentinel(t *testing.T) {
	err := &clusterDecommissionBlockedError{
		replicaID: 2,
		nodeID:    "node-2",
		stats:     nodePlacementStats{objects: 1},
	}

	if !errors.Is(err, errClusterDecommissionBlocked) {
		t.Fatal("expected blocked error to wrap sentinel")
	}
}
