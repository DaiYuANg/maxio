package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
	raftx "github.com/lyonbrown4d/maxio/internal/raft"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/object"
	"github.com/spf13/afero"
)

type lifecycleRaft struct {
	membership  raftx.Membership
	leaderErr   error
	addErr      error
	removeErr   error
	syncErr     error
	addCalls    int
	removeCalls int
	syncCalls   int
}

func newLifecycleRaft(nodes map[uint64]string) *lifecycleRaft {
	return &lifecycleRaft{
		membership: raftx.Membership{
			ConfigChangeID: 1,
			LocalReplicaID: 1,
			Nodes:          maps.Clone(nodes),
		},
	}
}

func (raft *lifecycleRaft) AddReplica(_ context.Context, replicaID uint64, target string) error {
	raft.addCalls++
	if raft.addErr != nil {
		return raft.addErr
	}
	if raft.membership.Nodes == nil {
		raft.membership.Nodes = map[uint64]string{}
	}
	raft.membership.Nodes[replicaID] = target
	raft.membership.ConfigChangeID++
	return nil
}

func (raft *lifecycleRaft) AssertLeader(context.Context) error {
	return raft.leaderErr
}

func (raft *lifecycleRaft) GetMembership(context.Context) (raftx.Membership, error) {
	return cloneLifecycleMembership(raft.membership), nil
}

func (raft *lifecycleRaft) LocalRaftAddress() string {
	return raft.membership.Nodes[raft.membership.LocalReplicaID]
}

func (raft *lifecycleRaft) LocalReplicaID() uint64 {
	return raft.membership.LocalReplicaID
}

func (raft *lifecycleRaft) RemoveReplica(_ context.Context, replicaID uint64) error {
	raft.removeCalls++
	if raft.removeErr != nil {
		return raft.removeErr
	}
	delete(raft.membership.Nodes, replicaID)
	raft.membership.Removed = append(raft.membership.Removed, replicaID)
	raft.membership.ConfigChangeID++
	return nil
}

func (raft *lifecycleRaft) SyncReplicas(
	_ context.Context,
	desired map[uint64]string,
) (raftx.SyncMembershipResult, error) {
	raft.syncCalls++
	before := cloneLifecycleMembership(raft.membership)
	if raft.syncErr != nil {
		return raftx.SyncMembershipResult{Before: before}, raft.syncErr
	}
	raft.membership.Nodes = maps.Clone(desired)
	raft.membership.ConfigChangeID++
	return raftx.SyncMembershipResult{
		Before: before,
		After:  cloneLifecycleMembership(raft.membership),
	}, nil
}

func cloneLifecycleMembership(membership raftx.Membership) raftx.Membership {
	return raftx.Membership{
		ConfigChangeID: membership.ConfigChangeID,
		LocalReplicaID: membership.LocalReplicaID,
		Nodes:          maps.Clone(membership.Nodes),
		NonVotings:     maps.Clone(membership.NonVotings),
		Witnesses:      maps.Clone(membership.Witnesses),
		Removed:        slices.Clone(membership.Removed),
	}
}

func newLifecycleService(t *testing.T, raft raftRuntime, objects ...model.ObjectMeta) *Service {
	t.Helper()
	return newService(Dependencies{
		objects: newLifecycleObjectService(t, objects...),
		engine:  newLifecycleEngine(t),
		raft:    raft,
	}, slog.New(slog.DiscardHandler), config.Config{}, nil)
}

func newLifecycleObjectService(t *testing.T, objects ...model.ObjectMeta) *object.Service {
	t.Helper()
	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	buckets := map[string]struct{}{}
	for index := range objects {
		objectMeta := objects[index]
		if _, ok := buckets[objectMeta.Bucket]; !ok {
			if err := meta.CreateBucket(ctx, objectMeta.Bucket); err != nil {
				t.Fatalf("create metadata bucket: %v", err)
			}
			buckets[objectMeta.Bucket] = struct{}{}
		}
		if err := meta.UpsertObjectMeta(ctx, objectMeta); err != nil {
			t.Fatalf("upsert object metadata: %v", err)
		}
	}
	storeModule, err := store.NewStore("", meta, newLifecycleEngine(t))
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return object.NewService(storeModule, nil, nil, slog.New(slog.DiscardHandler), config.Config{})
}

func newLifecycleEngine(t *testing.T) *engine.Engine {
	t.Helper()
	eng, err := engine.NewEngine("/lifecycle", engine.DefaultDataChunks, engine.DefaultParityChunks, afero.NewMemMapFs())
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return eng
}

func lifecyclePlacedObjects() []model.ObjectMeta {
	return []model.ObjectMeta{
		{
			Bucket:     "bucket",
			Key:        "object-a",
			ShardSizes: []int64{100, 200, 300},
			ShardPlacements: []model.ShardPlacement{
				{Index: 0, NodeID: "raft-2"},
				{Index: 1, NodeID: "raft-1"},
				{Index: 2, NodeID: "raft-2"},
			},
		},
		{
			Bucket:     "bucket",
			Key:        "object-b",
			ShardSizes: []int64{400},
			ShardPlacements: []model.ShardPlacement{
				{Index: 0, NodeID: "raft-2"},
			},
		},
	}
}

func decodeLifecycleJSON[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()
	var response T
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return response
}
