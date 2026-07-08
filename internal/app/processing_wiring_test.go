package app

import (
	"path/filepath"
	"testing"

	"github.com/arcgolabs/dix"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/processing"
)

func TestDefaultRuntimeWiresProcessingService(t *testing.T) {
	setRuntimeTestEnv(t)
	t.Setenv("MAXIO_PROCESSING_ENABLED", "false")

	app := newTestApp(t)
	service := dix.MustResolveAs[*processing.Service](app.Container())
	if service == nil {
		t.Fatal("expected processing service to be wired")
	}
	assertDisabledProcessingSnapshot(t, service.Snapshot())
}

func TestDefaultRuntimeWiresEnabledProcessingProcessors(t *testing.T) {
	setEnabledProcessingRuntimeEnv(t)
	app := newTestApp(t)
	service := dix.MustResolveAs[*processing.Service](app.Container())
	if service == nil {
		t.Fatal("expected processing service to be wired")
	}
	assertEnabledProcessingSnapshot(t, service.Snapshot())
}

func setEnabledProcessingRuntimeEnv(t *testing.T) {
	t.Helper()
	setRuntimeTestEnv(t)
	t.Setenv("MAXIO_PROCESSING_ENABLED", "true")
	t.Setenv("MAXIO_PROCESSING_MODE", processing.ModeAsyncPermissive)
	t.Setenv("MAXIO_PROCESSING_CLAMAV_ENABLED", "true")
	t.Setenv("MAXIO_PROCESSING_CLAMAV_MODE", processing.ModeInlineStrict)
	t.Setenv("MAXIO_PROCESSING_CLAMAV_ADDRESS", "127.0.0.1:3310")
	t.Setenv("MAXIO_PROCESSING_TIKA_ENABLED", "true")
	t.Setenv("MAXIO_PROCESSING_TIKA_MODE", processing.ModeAsyncPermissive)
	t.Setenv("MAXIO_PROCESSING_TIKA_FAIL_OPEN", "true")
	t.Setenv("MAXIO_PROCESSING_TIKA_URL", "http://127.0.0.1:9998")
}

func assertDisabledProcessingSnapshot(t *testing.T, snapshot processing.Snapshot) {
	t.Helper()
	if snapshot.Mode != processing.ModeDisabled {
		t.Fatalf("processing mode = %q, want %q", snapshot.Mode, processing.ModeDisabled)
	}
	if snapshot.ProcessorModes == nil {
		t.Fatal("expected processor modes map")
	}
	if snapshot.ProcessorFailOpen == nil {
		t.Fatal("expected processor fail-open map")
	}
}

func assertEnabledProcessingSnapshot(t *testing.T, snapshot processing.Snapshot) {
	t.Helper()
	if !snapshot.Enabled {
		t.Fatal("expected processing snapshot to be enabled")
	}
	assertProcessingProcessorModes(t, snapshot)
	assertProcessingCapabilities(t, snapshot)
}

func assertProcessingProcessorModes(t *testing.T, snapshot processing.Snapshot) {
	t.Helper()
	if snapshot.ProcessorModes["clamav"] != processing.ModeInlineStrict {
		t.Fatalf("clamav mode = %q, want %q", snapshot.ProcessorModes["clamav"], processing.ModeInlineStrict)
	}
	if snapshot.ProcessorModes["tika"] != processing.ModeAsyncPermissive {
		t.Fatalf("tika mode = %q, want %q", snapshot.ProcessorModes["tika"], processing.ModeAsyncPermissive)
	}
	if !snapshot.ProcessorFailOpen["tika"] {
		t.Fatalf("tika fail-open = %v, want true", snapshot.ProcessorFailOpen["tika"])
	}
	if !snapshotHasProcessor(snapshot, "clamav") || !snapshotHasProcessor(snapshot, "tika") {
		t.Fatalf("processors = %#v, want clamav and tika", snapshot.Processors.Values())
	}
}

func assertProcessingCapabilities(t *testing.T, snapshot processing.Snapshot) {
	t.Helper()
	if !snapshotHasCapability(snapshot, processing.CapabilityAntivirus) {
		t.Fatalf("capabilities = %#v, want antivirus", snapshot.Capabilities.Values())
	}
	if !snapshotHasCapability(snapshot, processing.CapabilityTextExtraction) || !snapshotHasCapability(snapshot, processing.CapabilityMetadataExtract) {
		t.Fatalf("capabilities = %#v, want tika text and metadata capabilities", snapshot.Capabilities.Values())
	}
}

func setRuntimeTestEnv(t *testing.T) {
	t.Helper()
	dataDir := t.TempDir()
	t.Setenv("MAXIO_DATA_DIR", dataDir)
	t.Setenv("MAXIO_METADATA_BACKEND", "sqlite")
	t.Setenv("MAXIO_METADATA_DSN", filepath.Join(dataDir, "metadata.db"))
	t.Setenv("MAXIO_METADATA_AUTO_MIGRATE", "true")
	t.Setenv("MAXIO_CACHE_BACKEND", "memory")
	t.Setenv("MAXIO_ENABLE_S3_PROXY", "false")
}

func newTestApp(t *testing.T) *App {
	t.Helper()
	app, err := New()
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	store := dix.MustResolveAs[metadata.MetadataStore](app.Container())
	if closer, ok := store.(interface{ Close() error }); ok {
		t.Cleanup(func() {
			if err := closer.Close(); err != nil {
				t.Fatalf("close metadata store: %v", err)
			}
		})
	}
	return app
}

func snapshotHasProcessor(snapshot processing.Snapshot, processor string) bool {
	if snapshot.Processors == nil {
		return false
	}
	for _, value := range snapshot.Processors.Values() {
		if value == processor {
			return true
		}
	}
	return false
}

func snapshotHasCapability(snapshot processing.Snapshot, capability processing.Capability) bool {
	if snapshot.Capabilities == nil {
		return false
	}
	for _, value := range snapshot.Capabilities.Values() {
		if value == string(capability) {
			return true
		}
	}
	return false
}
