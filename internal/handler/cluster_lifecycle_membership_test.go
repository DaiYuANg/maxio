package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	raftx "github.com/lyonbrown4d/maxio/internal/raft"
)

func TestClusterJoinBlocksRemovedReplicaReappearance(t *testing.T) {
	t.Parallel()

	raft := newLifecycleRaft(map[uint64]string{
		1: "127.0.0.1:63001",
	})
	raft.membership.Removed = []uint64{2}
	service := newLifecycleService(t, raft)
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		defaultClusterJoinPath,
		strings.NewReader(`{"replica_id":2,"target":"127.0.0.1:63002"}`),
	)
	recorder := httptest.NewRecorder()

	service.handleClusterJoin(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if raft.addCalls != 0 {
		t.Fatalf("add calls = %d, want 0", raft.addCalls)
	}
	response := decodeLifecycleJSON[clusterMembershipBlockedResponse](t, recorder)
	if response.Status != clusterMembershipStatusBlocked {
		t.Fatalf("status = %q, want blocked", response.Status)
	}
	if response.Reason != clusterMembershipReasonRemovedReplicaReappeared {
		t.Fatalf("reason = %q, want removed reappearance", response.Reason)
	}
	if response.ReplicaID != 2 {
		t.Fatalf("replica_id = %d, want 2", response.ReplicaID)
	}
}

func TestClusterMembersSyncBlocksAddressChange(t *testing.T) {
	t.Parallel()

	raft := newLifecycleRaft(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	service := newLifecycleService(t, raft)
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPut,
		defaultClusterMembersPath,
		strings.NewReader(`{"nodes":{"1":"127.0.0.1:63001","2":"127.0.0.1:63999"}}`),
	)
	recorder := httptest.NewRecorder()

	service.handleSyncClusterMembers(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if raft.syncCalls != 0 {
		t.Fatalf("sync calls = %d, want 0", raft.syncCalls)
	}
	response := decodeLifecycleJSON[clusterMembershipBlockedResponse](t, recorder)
	if response.Reason != clusterMembershipReasonAddressChangeBlocked {
		t.Fatalf("reason = %q, want address change blocked", response.Reason)
	}
	if response.Current != "127.0.0.1:63002" {
		t.Fatalf("current = %q, want original target", response.Current)
	}
	if response.Requested != "127.0.0.1:63999" {
		t.Fatalf("requested = %q, want changed target", response.Requested)
	}
}

func TestClusterBootstrapReturnsConflictDuringLeaderChange(t *testing.T) {
	t.Parallel()

	raft := newLifecycleRaft(map[uint64]string{
		1: "127.0.0.1:63001",
	})
	raft.syncErr = raftx.ErrNotLeader
	service := newLifecycleService(t, raft)
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		defaultClusterBootstrapPath,
		strings.NewReader(`{"nodes":{"1":"127.0.0.1:63001","2":"127.0.0.1:63002"}}`),
	)
	recorder := httptest.NewRecorder()

	service.handleClusterBootstrap(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if raft.syncCalls != 1 {
		t.Fatalf("sync calls = %d, want 1", raft.syncCalls)
	}
	response := decodeLifecycleJSON[map[string]string](t, recorder)
	if !strings.Contains(response["error"], raftx.ErrNotLeader.Error()) {
		t.Fatalf("error = %q, want not leader", response["error"])
	}
}

func TestDecommissionReturnsConflictWhenLeaderChangesBeforeRemove(t *testing.T) {
	t.Parallel()

	raft := newLifecycleRaft(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	raft.removeErr = raftx.ErrNotLeader
	service := newLifecycleService(t, raft)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/_cluster/members/2/decommission", http.NoBody)

	service.handleDecommissionClusterMember(recorder, request, 2)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if raft.removeCalls != 1 {
		t.Fatalf("remove calls = %d, want 1", raft.removeCalls)
	}
	response := decodeLifecycleJSON[map[string]string](t, recorder)
	if !strings.Contains(response["error"], raftx.ErrNotLeader.Error()) {
		t.Fatalf("error = %q, want not leader", response["error"])
	}
}

func TestClusterRebalanceIsIdempotentForEmptyMember(t *testing.T) {
	t.Parallel()

	raft := newLifecycleRaft(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	service := newLifecycleService(t, raft)

	result, err := service.rebalanceClusterMember(context.Background(), 2)

	if err != nil {
		t.Fatalf("rebalance empty cluster member: %v", err)
	}
	if result.Status != "already_balanced" {
		t.Fatalf("status = %q, want already_balanced", result.Status)
	}
	if result.Objects != 0 || result.Shards != 0 || result.UsedBytes != 0 {
		t.Fatalf("ownership = %d/%d/%d, want empty", result.Objects, result.Shards, result.UsedBytes)
	}
}
