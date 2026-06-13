package handler_test

import (
	"testing"

	"github.com/lyonbrown4d/maxio/internal/control"
	"github.com/lyonbrown4d/maxio/internal/handler"
)

func TestValidateClusterMemberDecommissionRejectsLocalReplica(t *testing.T) {
	t.Parallel()

	present, err := handler.ValidateClusterMemberDecommission(1, control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
		},
	})
	if err == nil {
		t.Fatal("expected local replica validation error")
	}
	if present {
		t.Fatal("local replica should not be considered decommissionable")
	}
}

func TestValidateClusterMemberDecommissionAllowsRemoteReplica(t *testing.T) {
	t.Parallel()

	present, err := handler.ValidateClusterMemberDecommission(2, control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
			2: "127.0.0.1:63001",
		},
	})
	if err != nil {
		t.Fatalf("validate remote replica: %v", err)
	}
	if !present {
		t.Fatal("remote replica should be reported as present")
	}
}

func TestValidateClusterMemberDecommissionTreatsMissingReplicaAsIdempotent(t *testing.T) {
	t.Parallel()

	present, err := handler.ValidateClusterMemberDecommission(3, control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "127.0.0.1:63000",
		},
	})
	if err != nil {
		t.Fatalf("validate missing replica: %v", err)
	}
	if present {
		t.Fatal("missing replica should not be reported as present")
	}
}
