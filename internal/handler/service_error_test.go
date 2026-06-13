package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/control"
	"github.com/lyonbrown4d/maxio/internal/object"
)

func TestWriteErrorReturnsConflictForControlNotLeader(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{}, slog.Default(), config.Config{})
	recorder := httptest.NewRecorder()
	service.writeError(recorder, control.ErrNotLeader)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusConflict)
	}
	content := recorder.Body.String()
	if !strings.Contains(content, "not leader") {
		t.Fatalf("error response = %s", content)
	}
}

func TestWriteErrorReturnsConflictForControlLeaderUnavailable(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{}, slog.Default(), config.Config{})
	recorder := httptest.NewRecorder()
	service.writeError(recorder, control.ErrLeaderUnavailable)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusConflict)
	}
	content := recorder.Body.String()
	if !strings.Contains(content, "leader unavailable") {
		t.Fatalf("error response = %s", content)
	}
}

func TestWriteErrorReturnsUnavailableForObjectCorruption(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{}, slog.Default(), config.Config{})
	recorder := httptest.NewRecorder()
	service.writeError(recorder, fmt.Errorf("read failed: %w", object.ErrObjectCorrupted))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	content := recorder.Body.String()
	if !strings.Contains(content, "object corrupted") {
		t.Fatalf("error response = %s", content)
	}
}

func TestWriteErrorReturnsUnavailableForShardRecoveryFailure(t *testing.T) {
	t.Parallel()

	service := NewService(Dependencies{}, slog.Default(), config.Config{})
	recorder := httptest.NewRecorder()
	service.writeError(recorder, fmt.Errorf("read failed: %w", object.ErrShardRecoveryFailed))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status code = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	content := recorder.Body.String()
	if !strings.Contains(content, "shard recovery failed") {
		t.Fatalf("error response = %s", content)
	}
}
