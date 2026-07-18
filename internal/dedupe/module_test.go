package dedupe

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/object"
)

func TestModule(t *testing.T) {
	module := Module()

	if module.Name() != "dedupe" {
		t.Fatalf("module name = %q, want %q", module.Name(), "dedupe")
	}
	if module.Disabled() {
		t.Fatal("module should not be disabled by default")
	}
}

func TestNewRuntimeDefaultsLoggerAndFields(t *testing.T) {
	cfg := config.Config{DedupeInterval: "10m", DedupeMaxFixes: 123}
	objects := &object.Service{}
	runtime := NewRuntime(cfg, objects, nil, nil)

	if runtime == nil {
		t.Fatal("runtime is nil")
	}
	if runtime.cfg.DedupeInterval != cfg.DedupeInterval {
		t.Fatalf("runtime cfg dedupe interval = %q, want %q", runtime.cfg.DedupeInterval, cfg.DedupeInterval)
	}
	if runtime.cfg.DedupeMaxFixes != cfg.DedupeMaxFixes {
		t.Fatalf("runtime cfg dedupe max fixes = %d, want %d", runtime.cfg.DedupeMaxFixes, cfg.DedupeMaxFixes)
	}
	if runtime.objects != objects {
		t.Fatalf("runtime objects = %p, want %p", runtime.objects, objects)
	}
	if runtime.logger == nil {
		t.Fatal("runtime logger is nil")
	}

	customLogger := slog.New(slog.DiscardHandler)
	runtimeWithLogger := NewRuntime(config.Config{}, objects, nil, customLogger)
	if runtimeWithLogger.logger != customLogger {
		t.Fatal("runtimeWithLogger should use provided logger")
	}
}

func TestStartRuntimeHookNoopWhenRuntimeNil(t *testing.T) {
	if err := startRuntime(context.Background(), nil); err != nil {
		t.Fatalf("startRuntime() = %v, want nil", err)
	}
}

func TestStartReturnsErrorForUnavailableScheduler(t *testing.T) {
	runtime := NewRuntime(config.Config{DedupeInterval: "1m", DedupeOnStart: true}, &object.Service{}, nil, nil)

	err := runtime.Start(context.Background())
	if err == nil {
		t.Fatal("start: expected error")
	}
	if !strings.Contains(err.Error(), "schedule dedupe job") {
		t.Fatalf("start error = %v, want scheduling error", err)
	}
	if !strings.Contains(err.Error(), "scheduler unavailable") {
		t.Fatalf("start error = %v, want scheduler unavailable", err)
	}

	if status := runtime.Status(); status.Running {
		t.Fatalf("status.running = true, want false")
	}
}

func TestPlanOnceAndRunOnceUnavailablePaths(t *testing.T) {
	t.Run("runtime nil", func(t *testing.T) {
		var runtime *Runtime
		if _, err := runtime.PlanOnce(context.Background()); err == nil {
			t.Fatal("plan once: expected error")
		}
		if _, err := runtime.RunOnce(context.Background()); err == nil {
			t.Fatal("run once: expected error")
		}
	})

	t.Run("objects missing", func(t *testing.T) {
		runtime := NewRuntime(config.Config{}, nil, nil, nil)
		if _, err := runtime.PlanOnce(context.Background()); err == nil {
			t.Fatal("plan once: expected error")
		}
		if _, err := runtime.RunOnce(context.Background()); err == nil {
			t.Fatal("run once: expected error")
		}
	})
}

func TestRunOnceUpdatesStatusOnFailure(t *testing.T) {
	runtime := NewRuntime(config.Config{}, &object.Service{}, nil, nil)

	_, err := runtime.RunOnce(context.Background())
	if err == nil {
		t.Fatal("run once: expected error")
	}

	status := runtime.Status()
	if status.Running {
		t.Fatalf("status.running = true, want false")
	}
	if status.LastStartedAt.IsZero() {
		t.Fatalf("status.last_started_at = %s, want populated", status.LastStartedAt)
	}
	if status.LastFinishedAt.Before(status.LastStartedAt) {
		t.Fatalf("status.last_finished_at = %s, want after last_started_at %s", status.LastFinishedAt, status.LastStartedAt)
	}
	if status.LastError != err.Error() {
		t.Fatalf("status.last_error = %q, want %q", status.LastError, err.Error())
	}
	if status.LastResult != (object.DedupeResult{}) {
		t.Fatalf("status.last_result = %+v, want zero value", status.LastResult)
	}
}
