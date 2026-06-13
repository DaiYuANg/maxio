package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/control"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
	"github.com/lyonbrown4d/maxio/object"
	"github.com/spf13/afero"
)

type lifecycleControl struct {
	membership  control.Membership
	leaderErr   error
	addErr      error
	removeErr   error
	syncErr     error
	addCalls    int
	removeCalls int
	syncCalls   int
}

func newLifecycleControl(nodes map[uint64]string) *lifecycleControl {
	return &lifecycleControl{
		membership: control.Membership{
			ConfigChangeID: 1,
			LocalReplicaID: 1,
			Nodes:          maps.Clone(nodes),
		},
	}
}

func (runtime *lifecycleControl) AddReplica(_ context.Context, replicaID uint64, target string) error {
	runtime.addCalls++
	if runtime.addErr != nil {
		return runtime.addErr
	}
	if runtime.membership.Nodes == nil {
		runtime.membership.Nodes = map[uint64]string{}
	}
	runtime.membership.Nodes[replicaID] = target
	runtime.membership.ConfigChangeID++
	return nil
}

func (runtime *lifecycleControl) AssertLeader(context.Context) error {
	return runtime.leaderErr
}

func (runtime *lifecycleControl) GetMembership(context.Context) (control.Membership, error) {
	return cloneLifecycleMembership(runtime.membership), nil
}

func (runtime *lifecycleControl) LocalControlAddress() string {
	return runtime.membership.Nodes[runtime.membership.LocalReplicaID]
}

func (runtime *lifecycleControl) LocalReplicaID() uint64 {
	return runtime.membership.LocalReplicaID
}

func (runtime *lifecycleControl) RemoveReplica(_ context.Context, replicaID uint64) error {
	runtime.removeCalls++
	if runtime.removeErr != nil {
		return runtime.removeErr
	}
	delete(runtime.membership.Nodes, replicaID)
	runtime.membership.Removed = append(runtime.membership.Removed, replicaID)
	runtime.membership.ConfigChangeID++
	return nil
}

func (runtime *lifecycleControl) SyncReplicas(
	_ context.Context,
	desired map[uint64]string,
) (control.SyncMembershipResult, error) {
	runtime.syncCalls++
	before := cloneLifecycleMembership(runtime.membership)
	if runtime.syncErr != nil {
		return control.SyncMembershipResult{Before: before}, runtime.syncErr
	}
	runtime.membership.Nodes = maps.Clone(desired)
	runtime.membership.ConfigChangeID++
	return control.SyncMembershipResult{
		Before: before,
		After:  cloneLifecycleMembership(runtime.membership),
	}, nil
}

func cloneLifecycleMembership(membership control.Membership) control.Membership {
	return control.Membership{
		ConfigChangeID: membership.ConfigChangeID,
		LocalReplicaID: membership.LocalReplicaID,
		Nodes:          maps.Clone(membership.Nodes),
		NonVotings:     maps.Clone(membership.NonVotings),
		Witnesses:      maps.Clone(membership.Witnesses),
		Removed:        slices.Clone(membership.Removed),
	}
}

func newLifecycleService(t *testing.T, runtime controlRuntime, objects ...model.ObjectMeta) *Service {
	t.Helper()
	return newService(Dependencies{
		objects: newLifecycleObjectService(t, objects...),
		engine:  newLifecycleEngine(t),
		control: runtime,
	}, slog.New(slog.DiscardHandler), config.Config{EnableClusterManagement: true}, nil)
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
	return object.NewService(storeModule, nil, nil, slog.New(slog.DiscardHandler), object.Config{})
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
				{Index: 0, NodeID: "node-2"},
				{Index: 1, NodeID: "node-1"},
				{Index: 2, NodeID: "node-2"},
			},
		},
		{
			Bucket:     "bucket",
			Key:        "object-b",
			ShardSizes: []int64{400},
			ShardPlacements: []model.ShardPlacement{
				{Index: 0, NodeID: "node-2"},
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
