package handler

import (
	"context"

	"github.com/lyonbrown4d/maxio/internal/control"
	"github.com/lyonbrown4d/maxio/internal/discovery"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/object"
)

type controlRuntime interface {
	AddReplica(ctx context.Context, replicaID uint64, target string) error
	AssertLeader(ctx context.Context) error
	GetMembership(ctx context.Context) (control.Membership, error)
	LocalControlAddress() string
	LocalReplicaID() uint64
	RemoveReplica(ctx context.Context, replicaID uint64) error
	SyncReplicas(ctx context.Context, desired map[uint64]string) (control.SyncMembershipResult, error)
}

// Dependencies groups handler dependencies to keep dix providers shallow.
type Dependencies struct {
	objects   *object.Service
	control   controlRuntime
	discovery *discovery.Runtime
	metadata  metadata.MetadataStore
}

// NewDependencies wires the handler dependency set.
func NewDependencies(discoveryRuntime *discovery.Runtime, metadataStore ...metadata.MetadataStore) Dependencies {
	var repo metadata.MetadataStore
	if len(metadataStore) > 0 {
		repo = metadataStore[0]
	}
	return Dependencies{discovery: discoveryRuntime, metadata: repo}
}
