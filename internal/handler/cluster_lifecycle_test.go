package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClusterBootstrapIsIdempotentForMatchingMembership(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	service := newLifecycleService(t, runtime)
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		defaultClusterBootstrapPath,
		strings.NewReader(`{"nodes":{"1":"127.0.0.1:63001","2":"127.0.0.1:63002"}}`),
	)
	recorder := httptest.NewRecorder()

	service.handleClusterBootstrap(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if runtime.syncCalls != 0 {
		t.Fatalf("sync calls = %d, want 0", runtime.syncCalls)
	}
	response := decodeLifecycleJSON[map[string]any](t, recorder)
	if response["status"] != "already_bootstrapped" {
		t.Fatalf("status = %v, want already_bootstrapped", response["status"])
	}
	if response["members"] != float64(2) {
		t.Fatalf("members = %v, want 2", response["members"])
	}
}

func TestClusterJoinIsIdempotentForExistingReplica(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	service := newLifecycleService(t, runtime)
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		defaultClusterJoinPath,
		strings.NewReader(`{"replica_id":2,"target":"127.0.0.1:63002"}`),
	)
	recorder := httptest.NewRecorder()

	service.handleClusterJoin(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusOK)
	}
	if runtime.addCalls != 0 {
		t.Fatalf("add calls = %d, want 0", runtime.addCalls)
	}
	response := decodeLifecycleJSON[map[string]any](t, recorder)
	if response["status"] != "already_joined" {
		t.Fatalf("status = %v, want already_joined", response["status"])
	}
}

func TestClusterJoinRejectsExistingReplicaTargetChange(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	service := newLifecycleService(t, runtime)
	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		defaultClusterJoinPath,
		strings.NewReader(`{"replica_id":2,"target":"127.0.0.1:63003"}`),
	)
	recorder := httptest.NewRecorder()

	service.handleClusterJoin(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if runtime.addCalls != 0 {
		t.Fatalf("add calls = %d, want 0", runtime.addCalls)
	}
	response := decodeLifecycleJSON[clusterMembershipBlockedResponse](t, recorder)
	if response.Status != clusterMembershipStatusBlocked {
		t.Fatalf("status = %q, want blocked", response.Status)
	}
	if response.Reason != clusterMembershipReasonAddressChangeBlocked {
		t.Fatalf("reason = %q, want address change blocked", response.Reason)
	}
	if !strings.Contains(response.Error, "already exists with different target") {
		t.Fatalf("error = %q, want target conflict", response.Error)
	}
}

func TestClusterMemberDeleteIsIdempotentForMissingReplica(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
	})
	service := newLifecycleService(t, runtime)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/_cluster/members/2", http.NoBody)
	recorder := httptest.NewRecorder()

	service.handleClusterMember(recorder, request, "2")

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusNoContent)
	}
	if runtime.removeCalls != 0 {
		t.Fatalf("remove calls = %d, want 0", runtime.removeCalls)
	}
}

func TestDecommissionClusterMemberIsIdempotentWhenReplicaAlreadyRemoved(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
	})
	service := newLifecycleService(t, runtime)

	result, err := service.decommissionClusterMember(context.Background(), 2)

	if err != nil {
		t.Fatalf("decommission missing member: %v", err)
	}
	if result.Status != "already_decommissioned" {
		t.Fatalf("status = %q, want already_decommissioned", result.Status)
	}
	if result.NodeID != "node-2" {
		t.Fatalf("node_id = %q, want node-2", result.NodeID)
	}
	if runtime.removeCalls != 0 {
		t.Fatalf("remove calls = %d, want 0", runtime.removeCalls)
	}
}

func TestClusterRebalancePlanReportsRemainingOwnershipStats(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	service := newLifecycleService(t, runtime, lifecyclePlacedObjects()...)

	plan, err := service.planClusterRebalance(context.Background(), 2)

	if err != nil {
		t.Fatalf("plan cluster rebalance: %v", err)
	}
	if plan.ReplicaID != 2 {
		t.Fatalf("replica_id = %d, want 2", plan.ReplicaID)
	}
	if plan.NodeID != "node-2" {
		t.Fatalf("node_id = %q, want node-2", plan.NodeID)
	}
	if plan.Objects != 2 {
		t.Fatalf("objects = %d, want 2", plan.Objects)
	}
	if plan.Shards != 3 {
		t.Fatalf("shards = %d, want 3", plan.Shards)
	}
	if plan.UsedBytes != 800 {
		t.Fatalf("used_bytes = %d, want 800", plan.UsedBytes)
	}
}

func TestDecommissionBlockedHTTPResponseIncludesRemainingOwnershipStats(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	service := newLifecycleService(t, runtime, lifecyclePlacedObjects()...)
	err := service.ensureClusterMemberDecommissionable(context.Background(), 2)
	if err == nil {
		t.Fatal("expected decommission blocked error")
	}
	recorder := httptest.NewRecorder()

	service.writeDecommissionError(recorder, err)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusConflict)
	}
	response := decodeLifecycleJSON[clusterMemberDecommissionBlockedResponse](t, recorder)
	if response.Status != "blocked" {
		t.Fatalf("status = %q, want blocked", response.Status)
	}
	if response.ReplicaID != 2 {
		t.Fatalf("replica_id = %d, want 2", response.ReplicaID)
	}
	if response.NodeID != "node-2" {
		t.Fatalf("node_id = %q, want node-2", response.NodeID)
	}
	if response.Objects != 2 {
		t.Fatalf("objects = %d, want 2", response.Objects)
	}
	if response.Shards != 3 {
		t.Fatalf("shards = %d, want 3", response.Shards)
	}
	if response.UsedBytes != 800 {
		t.Fatalf("used_bytes = %d, want 800", response.UsedBytes)
	}
}
