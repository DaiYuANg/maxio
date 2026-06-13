package handler

import (
	"testing"

	"github.com/lyonbrown4d/maxio/internal/control"
)

func TestResolveReplacementReplicaIDKeepsRequestedDifferentID(t *testing.T) {
	t.Parallel()

	membership := control.Membership{
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
			3: "127.0.0.1:63002",
		},
		Removed: []uint64{9},
	}
	got, err := resolveReplacementReplicaID(3, 12, membership)
	if err != nil {
		t.Fatalf("resolve replacement replica id: %v", err)
	}
	if got != 12 {
		t.Fatalf("replacement id = %d, want %d", got, 12)
	}
}

func TestResolveReplacementReplicaIDAutoAllocatesOnOldReplicaID(t *testing.T) {
	t.Parallel()

	membership := control.Membership{
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
			3: "127.0.0.1:63002",
		},
	}
	got, err := resolveReplacementReplicaID(3, 3, membership)
	if err != nil {
		t.Fatalf("resolve replacement replica id: %v", err)
	}
	if got != 4 {
		t.Fatalf("replacement id = %d, want 4", got)
	}
}

func TestResolveReplacementReplicaIDAutoAllocationSkipsInUseAndRemoved(t *testing.T) {
	t.Parallel()

	membership := control.Membership{
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
			5: "127.0.0.1:63001",
			7: "127.0.0.1:63002",
		},
		Removed: []uint64{8, 10},
	}
	got, err := resolveReplacementReplicaID(7, 7, membership)
	if err != nil {
		t.Fatalf("resolve replacement replica id: %v", err)
	}
	if got != 11 {
		t.Fatalf("replacement id = %d, want 11", got)
	}
}
