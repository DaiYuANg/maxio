package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/control"
	"github.com/lyonbrown4d/maxio/internal/dedupe"
	"github.com/lyonbrown4d/maxio/internal/repair"
	"github.com/lyonbrown4d/maxio/object"
)

func TestAddRepairStatusAddsSummaryMetrics(t *testing.T) {
	t.Parallel()

	collector := metricsCollector{}
	service := newService(Dependencies{}, slog.Default(), config.Config{}, nil)
	service.repair = &repair.Runtime{}

	collector.addRepairStatus(service)
	output := collector.String()

	required := []string{
		"maxio_repair_last_scrubbed 0",
		"maxio_repair_last_scrub_failed 0",
		"maxio_repair_last_checksum_failed 0",
		"maxio_repair_last_repair_attempts 0",
		"maxio_repair_last_repair_retries 0",
		"maxio_repair_last_retry_delay_ms 0",
		"maxio_repair_last_throttled 0",
		"maxio_repair_last_throttle_wait_ms 0",
		"maxio_repair_last_repaired_objects 0",
		"maxio_repair_last_unrecoverable 0",
		"maxio_repair_last_limited 0",
	}

	for _, name := range required {
		if !strings.Contains(output, name) {
			t.Fatalf("expected metric %q in output, got: %s", name, output)
		}
	}
}

func TestAddHTTPMetricsAddsRequestCounters(t *testing.T) {
	t.Parallel()

	collector := metricsCollector{}
	service := newService(Dependencies{}, slog.Default(), config.Config{}, nil)
	service.beginHTTPRequest()
	service.recordHTTPRequest(nil, http.StatusOK, 25*time.Millisecond)
	service.beginHTTPRequest()
	service.recordHTTPRequest(nil, http.StatusNotFound, 50*time.Millisecond)
	service.beginHTTPRequest()
	service.recordHTTPRequest(nil, http.StatusInternalServerError, 75*time.Millisecond)
	service.beginHTTPRequest()

	collector.addHTTPMetrics(service)
	output := collector.String()

	required := []string{
		"maxio_http_requests_total 3",
		"maxio_http_errors_total 2",
		"maxio_http_inflight_requests 1",
		"maxio_http_status_2xx_total 1",
		"maxio_http_status_4xx_total 1",
		"maxio_http_status_5xx_total 1",
		"maxio_http_request_duration_ms_total 150",
		"maxio_http_request_duration_ms_max 75",
	}

	for _, name := range required {
		if !strings.Contains(output, name) {
			t.Fatalf("expected metric %q in output, got: %s", name, output)
		}
	}
}

func TestAddControlStatusAddsLeaderAndMembershipMetrics(t *testing.T) {
	t.Parallel()

	collector := metricsCollector{}
	collector.addControlLeaderStatus(control.ErrNotLeader)
	collector.addControlMembershipMetrics(control.Membership{
		ConfigChangeID: 9,
		Nodes:          map[uint64]string{1: "node-a", 2: "node-b"},
		NonVotings:     map[uint64]string{3: "node-c"},
		Witnesses:      map[uint64]string{4: "node-d"},
		Removed:        []uint64{5},
	})
	output := collector.String()

	required := []string{
		"maxio_control_leader_available 1",
		"maxio_control_local_is_leader 0",
		"maxio_control_members 2",
		"maxio_control_removed_members 1",
		"maxio_control_non_voting_members 1",
		"maxio_control_witness_members 1",
		"maxio_control_config_change_id 9",
	}

	for _, name := range required {
		if !strings.Contains(output, name) {
			t.Fatalf("expected metric %q in output, got: %s", name, output)
		}
	}
}

func TestAddIndexStatusAddsSummaryMetrics(t *testing.T) {
	t.Parallel()

	collector := metricsCollector{}
	service := newService(Dependencies{objects: &object.Service{}}, slog.Default(), config.Config{}, nil)

	collector.addIndexStatus(service)
	output := collector.String()

	required := []string{
		"maxio_index_rebuilding 0",
		"maxio_index_queue_size 0",
		"maxio_index_queued_objects 0",
		"maxio_index_dropped_objects 0",
		"maxio_index_retried_objects 0",
		"maxio_index_indexed_objects 0",
		"maxio_index_failed_objects 0",
		"maxio_index_last_rebuild_objects 0",
		"maxio_index_last_rebuild_failed 0",
	}

	for _, name := range required {
		if !strings.Contains(output, name) {
			t.Fatalf("expected metric %q in output, got: %s", name, output)
		}
	}
}

func TestAddDedupeStatusAddsSummaryMetrics(t *testing.T) {
	t.Parallel()

	collector := metricsCollector{}
	service := newService(Dependencies{}, slog.Default(), config.Config{}, &dedupe.Runtime{})

	collector.addDedupeStatus(service)
	output := collector.String()

	required := []string{
		"maxio_dedupe_running 0",
		"maxio_dedupe_last_objects 0",
		"maxio_dedupe_last_blob_refs 0",
		"maxio_dedupe_last_hashes 0",
		"maxio_dedupe_last_fixes 0",
		"maxio_dedupe_last_ref_count_drift 0",
		"maxio_dedupe_last_missing_blob_refs 0",
		"maxio_dedupe_last_orphan_blob_refs 0",
		"maxio_dedupe_last_layouts_mismatched 0",
		"maxio_dedupe_last_bytes_reclaimable 0",
		"maxio_dedupe_last_bytes_reclaimed 0",
		"maxio_dedupe_last_limited 0",
	}

	for _, name := range required {
		if !strings.Contains(output, name) {
			t.Fatalf("expected metric %q in output, got: %s", name, output)
		}
	}
}

func TestAddRecoveryStatusAddsSummaryMetrics(t *testing.T) {
	t.Parallel()

	collector := metricsCollector{}
	service := newService(Dependencies{objects: &object.Service{}}, slog.Default(), config.Config{}, nil)

	collector.addRecoveryStatus(service)
	output := collector.String()

	required := []string{
		"maxio_recovery_completed 0",
		"maxio_recovery_last_failed 0",
		"maxio_recovery_last_dry_run 0",
		"maxio_recovery_last_pending_removed 0",
		"maxio_recovery_last_pending_wait 0",
		"maxio_recovery_last_pending_delete_staged 0",
		"maxio_recovery_last_pending_rollback_layout 0",
		"maxio_recovery_last_pending_release_blob 0",
		"maxio_recovery_last_pending_committed_cleanup 0",
		"maxio_recovery_last_orphan_shards_scanned 0",
		"maxio_recovery_last_orphan_shards_removed 0",
		"maxio_recovery_last_orphan_shards 0",
	}

	for _, name := range required {
		if !strings.Contains(output, name) {
			t.Fatalf("expected metric %q in output, got: %s", name, output)
		}
	}
}
