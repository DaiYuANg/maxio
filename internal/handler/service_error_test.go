package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/object"
)

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
