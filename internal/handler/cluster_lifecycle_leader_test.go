package handler

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/control"
)

func TestReadinessReportsLeaderUnavailableDiagnostic(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	runtime.leaderErr = control.ErrLeaderUnavailable
	service := newLifecycleService(t, runtime)

	response := service.readiness(context.Background())

	if response.Status != "not_ready" {
		t.Fatalf("status = %q, want not_ready", response.Status)
	}
	if response.Checks["control_membership"] != "ok" {
		t.Fatalf("control_membership = %q, want ok", response.Checks["control_membership"])
	}
	if response.Checks["control_leader"] != control.ErrLeaderUnavailable.Error() {
		t.Fatalf("control_leader = %q, want %q", response.Checks["control_leader"], control.ErrLeaderUnavailable.Error())
	}
}

func TestReadinessTreatsRemoteLeaderAsReadyDiagnostic(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	runtime.leaderErr = fmt.Errorf("%w: leader=2 local=1", control.ErrNotLeader)
	service := newLifecycleService(t, runtime)

	response := service.readiness(context.Background())

	if response.Status != "ok" {
		t.Fatalf("status = %q, want ok", response.Status)
	}
	if response.Checks["control_leader"] != "remote" {
		t.Fatalf("control_leader = %q, want remote", response.Checks["control_leader"])
	}
}

func TestClusterMetricsReportsLeaderUnavailableAndMembership(t *testing.T) {
	t.Parallel()

	runtime := newLifecycleControl(map[uint64]string{
		1: "127.0.0.1:63001",
		2: "127.0.0.1:63002",
	})
	runtime.leaderErr = control.ErrLeaderUnavailable
	service := newLifecycleService(t, runtime)
	collector := metricsCollector{}

	collector.addControlStatus(context.Background(), service)

	output := collector.String()
	required := []string{
		"maxio_control_local_replica_id 1",
		"maxio_control_leader_available 0",
		"maxio_control_local_is_leader 0",
		"maxio_control_members 2",
	}
	for _, metric := range required {
		if !strings.Contains(output, metric) {
			t.Fatalf("expected metric %q in output, got: %s", metric, output)
		}
	}
}
