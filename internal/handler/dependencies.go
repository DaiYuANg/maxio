package handler

import (
	"context"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/discovery"
	raftx "github.com/lyonbrown4d/maxio/internal/raft"
	"github.com/lyonbrown4d/maxio/internal/repair"
	"github.com/lyonbrown4d/maxio/object"
	maxios3 "github.com/lyonbrown4d/maxio/s3"
)

type raftRuntime interface {
	AddReplica(ctx context.Context, replicaID uint64, target string) error
	AssertLeader(ctx context.Context) error
	GetMembership(ctx context.Context) (raftx.Membership, error)
	LocalRaftAddress() string
	LocalReplicaID() uint64
	RemoveReplica(ctx context.Context, replicaID uint64) error
	SyncReplicas(ctx context.Context, desired map[uint64]string) (raftx.SyncMembershipResult, error)
}

// Dependencies groups handler dependencies to keep dix providers shallow.
type Dependencies struct {
	objects   *object.Service
	engine    *engine.Engine
	raft      raftRuntime
	discovery *discovery.Runtime
	s3        *maxios3.Service
	repair    *repair.Runtime
}

// NewDependencies wires the handler dependency set.
func NewDependencies(
	objects *object.Service,
	engineStore *engine.Engine,
	raftRT *raftx.Runtime,
	discoveryRuntime *discovery.Runtime,
	s3Service *maxios3.Service,
	repairRuntime *repair.Runtime,
) Dependencies {
	var raftDep raftRuntime
	if raftRT != nil {
		raftDep = raftRT
	}
	return Dependencies{
		objects:   objects,
		engine:    engineStore,
		raft:      raftDep,
		discovery: discoveryRuntime,
		s3:        s3Service,
		repair:    repairRuntime,
	}
}
