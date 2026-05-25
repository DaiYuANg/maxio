package handler

import (
	"errors"
	"testing"

	raftx "github.com/lyonbrown4d/maxio/internal/raft"
	"github.com/lyonbrown4d/maxio/model"
)

func TestValidateClusterMemberRebalanceAcceptsPresentReplica(t *testing.T) {
	err := ValidateClusterMemberRebalance(2, raftx.Membership{
		Nodes: map[uint64]string{1: "localhost:6301", 2: "localhost:6302"},
	})

	if err != nil {
		t.Fatalf("validate rebalance replica: %v", err)
	}
}

func TestValidateClusterMemberRebalanceRejectsZeroReplica(t *testing.T) {
	err := ValidateClusterMemberRebalance(0, raftx.Membership{})

	if err == nil {
		t.Fatal("expected zero replica validation error")
	}
}

func TestValidateClusterMemberRebalanceRejectsMissingReplica(t *testing.T) {
	err := ValidateClusterMemberRebalance(3, raftx.Membership{
		Nodes: map[uint64]string{1: "localhost:6301", 2: "localhost:6302"},
	})

	if !errors.Is(err, errClusterRebalanceMemberNotFound) {
		t.Fatalf("expected missing member error, got %v", err)
	}
}

func TestCountObjectPlacementsIncludesUsedBytes(t *testing.T) {
	objects := []model.ObjectMeta{
		{
			Key:        "object-a",
			ShardSizes: []int64{10, 20, 30},
			ShardPlacements: []model.ShardPlacement{
				{Index: 0, NodeID: "node-a"},
				{Index: 1, NodeID: "node-b"},
				{Index: 2, NodeID: "node-a"},
			},
		},
		{
			Key:        "object-b",
			ShardSizes: []int64{40},
			ShardPlacements: []model.ShardPlacement{
				{Index: 0, NodeID: "node-a"},
			},
		},
	}

	stats := countObjectPlacements(objects, "node-a")

	if stats.objects != 2 {
		t.Fatalf("objects = %d, want 2", stats.objects)
	}
	if stats.shards != 3 {
		t.Fatalf("shards = %d, want 3", stats.shards)
	}
	if stats.usedBytes != 80 {
		t.Fatalf("usedBytes = %d, want 80", stats.usedBytes)
	}
	if !stats.hasPlacements() {
		t.Fatal("expected non-empty placement stats")
	}
}

func TestCountObjectPlacementsIgnoresMissingShardSizes(t *testing.T) {
	objects := []model.ObjectMeta{
		{
			Key:        "object-a",
			ShardSizes: []int64{10},
			ShardPlacements: []model.ShardPlacement{
				{Index: 0, NodeID: "node-a"},
				{Index: 2, NodeID: "node-a"},
				{Index: -1, NodeID: "node-a"},
			},
		},
	}

	stats := countObjectPlacements(objects, "node-a")

	if stats.objects != 1 {
		t.Fatalf("objects = %d, want 1", stats.objects)
	}
	if stats.shards != 3 {
		t.Fatalf("shards = %d, want 3", stats.shards)
	}
	if stats.usedBytes != 10 {
		t.Fatalf("usedBytes = %d, want 10", stats.usedBytes)
	}
}

func TestCountObjectPlacementsEmptyStats(t *testing.T) {
	stats := countObjectPlacements([]model.ObjectMeta{
		{
			Key: "object-a",
			ShardPlacements: []model.ShardPlacement{
				{Index: 0, NodeID: "node-a"},
			},
		},
	}, "node-b")

	if stats.hasPlacements() {
		t.Fatal("expected empty placement stats")
	}
}
