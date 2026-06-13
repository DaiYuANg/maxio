package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/discovery"
)

func TestParseRequiredReplicaIDMissingParam(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/?_", http.NoBody)
	_, err := parseRequiredReplicaID(request)
	if err == nil {
		t.Fatal("expected missing replica_id error")
	}
}

func TestParseRequiredReplicaIDRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/?replica_id=abc", http.NoBody)
	_, err := parseRequiredReplicaID(request)
	if err == nil {
		t.Fatal("expected invalid replica_id error")
	}
}

func TestParseRequiredReplicaIDRejectsZero(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/?replica_id=0", http.NoBody)
	_, err := parseRequiredReplicaID(request)
	if err == nil {
		t.Fatal("expected zero replica_id error")
	}
}

func TestParseRequiredReplicaIDParsesValidValue(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/?replica_id=12", http.NoBody)
	replicaID, err := parseRequiredReplicaID(request)
	if err != nil {
		t.Fatalf("parse replica_id: %v", err)
	}
	if replicaID != 12 {
		t.Fatalf("replica_id = %d, want %d", replicaID, 12)
	}
}

func TestParseBoolQueryDefaultsToFalse(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/?_a=1", http.NoBody)
	value, err := parseBoolQuery(request, "enabled")
	if err != nil {
		t.Fatalf("parse bool query: %v", err)
	}
	if value {
		t.Fatal("expected default false value")
	}
}

func TestParseBoolQueryRejectsInvalidValue(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/?enabled=maybe", http.NoBody)
	_, err := parseBoolQuery(request, "enabled")
	if err == nil {
		t.Fatal("expected invalid bool query error")
	}
}

func TestParseBoolQueryAcceptsCommonValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw      string
		expected bool
	}{
		{raw: "enabled=1", expected: true},
		{raw: "enabled=true", expected: true},
		{raw: "enabled=FALSE", expected: false},
	}
	for _, tc := range tests {
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/?"+tc.raw, http.NoBody)
		value, err := parseBoolQuery(request, "enabled")
		if err != nil {
			t.Fatalf("parse bool query %q: %v", tc.raw, err)
		}
		if value != tc.expected {
			t.Fatalf("parse bool query %q = %v, want %v", tc.raw, value, tc.expected)
		}
	}
}

func TestUsableDiscoveredNodeRequiresAliveAndTarget(t *testing.T) {
	t.Parallel()

	node := discovery.Node{
		ReplicaID:      1,
		State:          "alive",
		ControlAddress: "127.0.0.1:63000",
	}
	if !usableDiscoveredNode(node) {
		t.Fatal("expected alive discovered node with control address to be usable")
	}

	node.State = "dead"
	if usableDiscoveredNode(node) {
		t.Fatal("dead discovered node should not be usable")
	}

	node.State = "alive"
	node.ControlAddress = ""
	if usableDiscoveredNode(node) {
		t.Fatal("discovered node without control address should not be usable")
	}
}

func TestStorageReplicaIDParsesNumericID(t *testing.T) {
	t.Parallel()

	replicaID, ok := storageReplicaID("node-42")
	if !ok || replicaID != 42 {
		t.Fatalf("storageReplicaID = %d, %t, want 42, true", replicaID, ok)
	}
}

func TestStorageReplicaIDRejectsInvalidNodeID(t *testing.T) {
	t.Parallel()

	replicaID, ok := storageReplicaID("storage-99")
	if ok || replicaID != 0 {
		t.Fatalf("storageReplicaID = %d, %t, want 0, false", replicaID, ok)
	}
}

func TestIsClusterRouteChecks(t *testing.T) {
	t.Parallel()

	if !isClusterMemberRoute([]string{"_cluster", "members", "5"}) {
		t.Fatal("expected members route to match")
	}
	if isClusterMemberRoute([]string{"_cluster", "members"}) {
		t.Fatal("expected non-member-id path not to match")
	}
	if !isClusterMemberActionRoute([]string{"_cluster", "members", "5", "drain"}) {
		t.Fatal("expected member action route to match")
	}
	if isClusterMemberActionRoute([]string{"_cluster", "members", "5"}) {
		t.Fatal("expected incomplete action path not to match")
	}
}

func TestMembershipStatesMatch(t *testing.T) {
	t.Parallel()

	current := map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	}
	desired := map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	}
	if !membershipStatesMatch(current, desired) {
		t.Fatal("expected membership maps to match")
	}
}

func TestMembershipStatesMatchRejectsDifferentTarget(t *testing.T) {
	t.Parallel()

	current := map[uint64]string{
		1: "127.0.0.1:63001",
	}
	desired := map[uint64]string{
		1: "127.0.0.1:63003",
	}
	if membershipStatesMatch(current, desired) {
		t.Fatal("expected different target to not match")
	}
}

func TestMembershipStatesMatchRejectsDifferentSize(t *testing.T) {
	t.Parallel()

	current := map[uint64]string{
		1: "127.0.0.1:63001",
	}
	desired := map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	}
	if membershipStatesMatch(current, desired) {
		t.Fatal("expected different size to not match")
	}
}
