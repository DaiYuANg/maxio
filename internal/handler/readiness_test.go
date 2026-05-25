package handler

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/repair"
	"github.com/lyonbrown4d/maxio/object"
)

func TestReadinessReportsStorageWritableAndRepairBacklog(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	service := newService(Dependencies{
		objects: &object.Service{},
		engine:  eng,
		repair:  &repair.Runtime{},
	}, slog.New(slog.DiscardHandler), config.Config{}, nil)

	response := service.readiness(context.Background())
	if response.Checks["storage_writable"] != "ok" {
		t.Fatalf("storage_writable = %q, want ok", response.Checks["storage_writable"])
	}
	if response.Checks["repair_backlog"] != "ok" {
		t.Fatalf("repair_backlog = %q, want ok", response.Checks["repair_backlog"])
	}
	if response.Checks["raft_membership"] != "unavailable" {
		t.Fatalf("raft_membership = %q, want unavailable", response.Checks["raft_membership"])
	}
	if response.Checks["raft_leader"] != "unavailable" {
		t.Fatalf("raft_leader = %q, want unavailable", response.Checks["raft_leader"])
	}
	if response.Status != "not_ready" {
		t.Fatalf("status = %q, want not_ready", response.Status)
	}
}

func TestReadinessReportsNoWritableStorageNodes(t *testing.T) {
	t.Parallel()

	eng, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, nil)
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	if err := eng.DrainStorageNode(engine.DefaultLocalNodeID); err != nil {
		t.Fatalf("drain local node: %v", err)
	}
	service := newService(Dependencies{
		objects: &object.Service{},
		engine:  eng,
		repair:  &repair.Runtime{},
	}, slog.New(slog.DiscardHandler), config.Config{}, nil)

	response := service.readiness(context.Background())
	if response.Checks["storage_writable"] != "no_writable_storage_nodes" {
		t.Fatalf("storage_writable = %q, want no_writable_storage_nodes", response.Checks["storage_writable"])
	}
}
