package handler

import (
	"errors"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/control"
)

func TestValidateClusterMemberReplacementAcceptsRemoteReplica(t *testing.T) {
	err := ValidateClusterMemberReplacement(2, control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "localhost:6301",
			2: "localhost:6302",
		},
	})

	if err != nil {
		t.Fatalf("validate replacement: %v", err)
	}
}

func TestValidateClusterMemberReplacementRejectsZeroReplica(t *testing.T) {
	err := ValidateClusterMemberReplacement(0, control.Membership{})

	if err == nil {
		t.Fatal("expected zero replica validation error")
	}
}

func TestValidateClusterMemberReplacementRejectsLocalReplica(t *testing.T) {
	err := ValidateClusterMemberReplacement(1, control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "localhost:6301",
			2: "localhost:6302",
		},
	})

	if !errors.Is(err, errCannotReplaceLocalReplica) {
		t.Fatalf("expected local replica error, got %v", err)
	}
}

func TestValidateClusterMemberReplacementRejectsMissingReplica(t *testing.T) {
	err := ValidateClusterMemberReplacement(3, control.Membership{
		LocalReplicaID: 1,
		Nodes: map[uint64]string{
			1: "localhost:6301",
			2: "localhost:6302",
		},
	})

	if !errors.Is(err, errClusterReplaceMemberNotFound) {
		t.Fatalf("expected missing member error, got %v", err)
	}
}
