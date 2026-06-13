package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

type metricsCollector struct {
	builder          strings.Builder
	collectionErrors int
}

func (s *Service) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	output := s.collectMetrics(r.Context())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(output)); err != nil {
		s.logger.WarnContext(r.Context(), "write metrics response failed", "error", err)
	}
}

func (s *Service) collectMetrics(ctx context.Context) string {
	collector := metricsCollector{}
	collector.addHTTPMetrics(s)
	collector.addReadiness(ctx, s)
	collector.addStorageNodes(s)
	collector.addObjectCounts(ctx, s)
	collector.addControlStatus(ctx, s)
	collector.addRepairStatus(s)
	collector.addDedupeStatus(s)
	collector.addIndexStatus(s)
	collector.addRecoveryStatus(s)
	collector.gauge("maxio_metrics_collection_errors", "Number of metric collection failures.", collector.collectionErrors)
	return collector.String()
}

func (collector *metricsCollector) addReadiness(ctx context.Context, s *Service) {
	value := 0
	if s.readiness(ctx).Status == "ok" {
		value = 1
	}
	collector.gauge("maxio_ready", "Whether MaxIO is ready to serve traffic.", value)
}

func (collector *metricsCollector) addStorageNodes(s *Service) {
	_ = s
	collector.gauge("maxio_storage_nodes", "Configured storage nodes.", 0)
	collector.gauge("maxio_storage_nodes_drained", "Storage nodes excluded from new placements.", 0)
	collector.gauge("maxio_storage_node_objects", "Objects assigned to storage nodes.", 0)
	collector.gauge("maxio_storage_node_shards", "Shards assigned to storage nodes.", 0)
	collector.gaugeInt64("maxio_storage_node_used_bytes", "Bytes assigned to storage nodes.", 0)
}

func (collector *metricsCollector) addObjectCounts(ctx context.Context, s *Service) {
	_ = ctx
	if s == nil || s.metadata == nil {
		collector.gauge("maxio_buckets", "Buckets known to metadata.", 0)
		collector.gauge("maxio_objects", "Committed objects known to metadata.", 0)
		return
	}
	buckets, err := s.metadata.ListBuckets(ctx)
	if err != nil {
		collector.collectionErrors++
		collector.gauge("maxio_buckets", "Buckets known to metadata.", 0)
		collector.gauge("maxio_objects", "Committed objects known to metadata.", 0)
		return
	}
	objects := 0
	for _, bucket := range buckets {
		items, err := s.metadata.ListObjectMetas(ctx, bucket.Name, "")
		if err != nil {
			collector.collectionErrors++
			continue
		}
		objects += len(items)
	}
	collector.gauge("maxio_buckets", "Buckets known to metadata.", len(buckets))
	collector.gauge("maxio_objects", "Committed objects known to metadata.", objects)
}

func (collector *metricsCollector) addRepairStatus(s *Service) {
	_ = s
	collector.gauge("maxio_repair_running", "Whether the repair job is running.", 0)
	collector.gauge("maxio_repair_last_objects", "Objects scanned by the last repair job.", 0)
	collector.gauge("maxio_repair_last_unhealthy", "Unhealthy objects found by the last repair job.", 0)
	collector.gauge("maxio_repair_last_repaired_objects", "Objects repaired by the last repair job.", 0)
	collector.gauge("maxio_repair_last_failed", "Failures recorded by the last repair job.", 0)
}

func (collector *metricsCollector) addDedupeStatus(s *Service) {
	if s == nil || s.dedupe == nil {
		collector.gauge("maxio_dedupe_running", "Whether the dedupe job is running.", 0)
		return
	}
	status := s.dedupe.Status()
	result := status.LastResult
	collector.gauge("maxio_dedupe_running", "Whether the dedupe job is running.", boolInt(status.Running))
	collector.gauge("maxio_dedupe_last_objects", "Objects scanned by the last dedupe job.", result.Objects)
	collector.gauge("maxio_dedupe_last_blob_refs", "Blob refs scanned by the last dedupe job.", result.BlobRefs)
	collector.gauge("maxio_dedupe_last_hashes", "Unique object hashes seen by the last dedupe job.", result.Hashes)
	collector.gauge("maxio_dedupe_last_fixes", "Fixes applied by the last dedupe job.", result.Fixes)
	collector.gauge("maxio_dedupe_last_ref_count_drift", "Blob ref count drift found by the last dedupe job.", result.RefCountDrift)
	collector.gauge("maxio_dedupe_last_missing_blob_refs", "Missing blob refs found by the last dedupe job.", result.MissingBlobRefs)
	collector.gauge("maxio_dedupe_last_orphan_blob_refs", "Orphan blob refs found by the last dedupe job.", result.OrphanBlobRefs)
	collector.gauge("maxio_dedupe_last_layouts_mismatched", "Object layouts mismatched by the last dedupe job.", result.LayoutsMismatched)
	collector.gaugeInt64("maxio_dedupe_last_bytes_reclaimable", "Bytes reclaimable found by the last dedupe job.", result.BytesReclaimable)
	collector.gaugeInt64("maxio_dedupe_last_bytes_reclaimed", "Bytes reclaimed by the last dedupe job.", result.BytesReclaimed)
	collector.gauge("maxio_dedupe_last_limited", "Whether the last dedupe job was limited by configured thresholds.", boolInt(result.Limited))
}

func (collector *metricsCollector) addIndexStatus(s *Service) {
	if s == nil || s.objects == nil {
		collector.gauge("maxio_index_rebuilding", "Whether the content index rebuild is running.", 0)
		return
	}
	status := s.objects.IndexStatus()
	collector.gauge("maxio_index_rebuilding", "Whether the content index rebuild is running.", boolInt(status.Rebuilding))
	collector.gauge("maxio_index_queue_size", "Configured content index queue size.", status.QueueSize)
	collector.gauge("maxio_index_queued_objects", "Objects waiting in the content index queue.", status.QueuedObjects)
	collector.gauge("maxio_index_dropped_objects", "Object index events dropped because the queue was full.", status.DroppedObjects)
	collector.gauge("maxio_index_retried_objects", "Object index tasks retried after failures.", status.RetriedObjects)
	collector.gauge("maxio_index_indexed_objects", "Objects successfully indexed by the content index worker.", status.IndexedObjects)
	collector.gauge("maxio_index_failed_objects", "Objects that failed content indexing.", status.FailedObjects)
	collector.gauge("maxio_index_last_rebuild_objects", "Objects indexed by the last content index rebuild.", status.LastRebuildObjects)
	collector.gauge("maxio_index_last_rebuild_failed", "Objects that failed during the last content index rebuild.", status.LastRebuildFailed)
}

func (collector *metricsCollector) addRecoveryStatus(s *Service) {
	_ = s
	collector.gauge("maxio_recovery_completed", "Whether storage recovery has completed at least once.", 0)
	collector.gauge("maxio_recovery_last_failed", "Whether the last storage recovery run failed.", 0)
	collector.gauge("maxio_recovery_last_pending_removed", "Pending objects removed by the last storage recovery run.", 0)
	collector.gauge("maxio_recovery_last_orphan_shards_removed", "Orphan shard sets removed by the last storage recovery run.", 0)
}

func (collector *metricsCollector) gauge(name, help string, value int) {
	collector.gaugeInt64(name, help, int64(value))
}

func (collector *metricsCollector) gaugeInt64(name, help string, value int64) {
	collector.line("# HELP " + name + " " + help)
	collector.line("# TYPE " + name + " gauge")
	collector.line(name + " " + formatMetricInt(value))
}

func (collector *metricsCollector) gaugeUint64(name, help string, value uint64) {
	collector.line("# HELP " + name + " " + help)
	collector.line("# TYPE " + name + " gauge")
	collector.line(name + " " + strconv.FormatUint(value, 10))
}

func formatMetricInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func (collector *metricsCollector) line(value string) {
	if _, err := collector.builder.WriteString(value); err != nil {
		collector.collectionErrors++
	}
	if err := collector.builder.WriteByte('\n'); err != nil {
		collector.collectionErrors++
	}
}

func (collector *metricsCollector) String() string {
	return collector.builder.String()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
