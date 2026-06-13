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
	}, slog.New(slog.DiscardHandler), config.Config{EnableNativeObjectAPI: true}, nil)

	response := service.readiness(context.Background())
	if response.Checks["storage_writable"] != "ok" {
		t.Fatalf("storage_writable = %q, want ok", response.Checks["storage_writable"])
	}
	if response.Checks["repair_backlog"] != "ok" {
		t.Fatalf("repair_backlog = %q, want ok", response.Checks["repair_backlog"])
	}
	if response.Checks["control_membership"] != "disabled" {
		t.Fatalf("control_membership = %q, want disabled", response.Checks["control_membership"])
	}
	if response.Checks["control_leader"] != "disabled" {
		t.Fatalf("control_leader = %q, want disabled", response.Checks["control_leader"])
	}
	if response.Status != "ok" {
		t.Fatalf("status = %q, want ok", response.Status)
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
	}, slog.New(slog.DiscardHandler), config.Config{EnableNativeObjectAPI: true}, nil)

	response := service.readiness(context.Background())
	if response.Checks["storage_writable"] != "no_writable_storage_nodes" {
		t.Fatalf("storage_writable = %q, want no_writable_storage_nodes", response.Checks["storage_writable"])
	}
}
