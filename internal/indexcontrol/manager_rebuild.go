package indexcontrol

import (
	"context"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	searchindex "github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const maxIndexManagerErrorLength = 2048

type objectRebuildResult struct {
	indexed bool
	failed  bool
	stop    bool
	message string
}

func (manager *Manager) listCommittedObjectMetas(ctx context.Context) (*collectionlist.List[model.ObjectMeta], error) {
	buckets, err := manager.metadata.ListBuckets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list buckets for index rebuild: %w", err)
	}
	if buckets == nil {
		return collectionlist.NewList[model.ObjectMeta](), nil
	}
	objects, err := collectionlist.ReduceErrList(
		buckets,
		collectionlist.NewList[model.ObjectMeta](),
		func(objects *collectionlist.List[model.ObjectMeta], _ int, bucket model.Bucket) (*collectionlist.List[model.ObjectMeta], error) {
			metas, listErr := manager.metadata.ListObjectMetas(ctx, bucket.Name, "")
			if listErr != nil {
				return objects, fmt.Errorf("list %q object metadata: %w", bucket.Name, listErr)
			}
			if metas != nil {
				objects.MergeSlice(metas.Values())
			}
			return objects, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list committed object metadata: %w", err)
	}
	return objects, nil
}
func (manager *Manager) rebuildObjects(
	ctx context.Context,
	objects *collectionlist.List[model.ObjectMeta],
	now func() time.Time,
) (int, int, string) {
	if objects == nil {
		return 0, 0, ""
	}
	indexed := 0
	failed := 0
	message := ""
	objects.Range(func(_ int, meta model.ObjectMeta) bool {
		result := manager.rebuildObject(ctx, meta, now)
		if result.indexed {
			indexed++
		}
		if result.failed {
			failed++
		}
		message = firstMessage(message, result.message)
		return !result.stop
	})
	return indexed, failed, message
}

func (manager *Manager) rebuildObject(ctx context.Context, meta model.ObjectMeta, now func() time.Time) objectRebuildResult {
	if err := ctx.Err(); err != nil {
		return objectRebuildResult{stop: true, message: failureMessage(err)}
	}
	versionID, err := manager.currentVersionID(ctx, meta)
	if err != nil {
		return objectRebuildResult{failed: true, message: failureMessage(err)}
	}
	count, err := manager.search.UpsertDocuments([]searchindex.IndexDocument{{Meta: meta}})
	if err != nil || count == 0 {
		return manager.failedObjectRebuild(ctx, meta, versionID, err)
	}
	if err := manager.recordIndexDocument(ctx, meta, versionID, model.IndexDocumentStateIndexed, now(), nil); err != nil {
		return objectRebuildResult{failed: true, message: failureMessage(err)}
	}
	return objectRebuildResult{indexed: true}
}

func (manager *Manager) failedObjectRebuild(
	ctx context.Context,
	meta model.ObjectMeta,
	versionID string,
	cause error,
) objectRebuildResult {
	if cause == nil {
		cause = fmt.Errorf("search index accepted no documents for %s/%s", meta.Bucket, meta.Key)
	}
	message := failureMessage(cause)
	if err := manager.recordIndexDocument(ctx, meta, versionID, model.IndexDocumentStateFailed, time.Time{}, cause); err != nil {
		message = firstErrorMessage(message, err)
	}
	return objectRebuildResult{failed: true, message: message}
}

func (manager *Manager) currentVersionID(ctx context.Context, meta model.ObjectMeta) (string, error) {
	record, found, err := manager.metadata.GetObjectRecord(ctx, meta.Bucket, meta.Key)
	if err != nil {
		return "", fmt.Errorf("get object record %s/%s: %w", meta.Bucket, meta.Key, err)
	}
	if found && strings.TrimSpace(record.CurrentVersionID) != "" {
		return strings.TrimSpace(record.CurrentVersionID), nil
	}
	return manager.currentVersionIDFromVersions(ctx, meta)
}

func (manager *Manager) currentVersionIDFromVersions(ctx context.Context, meta model.ObjectMeta) (string, error) {
	versions, err := manager.metadata.ListObjectVersions(ctx, meta.Bucket, meta.Key)
	if err != nil {
		return "", fmt.Errorf("list object versions %s/%s: %w", meta.Bucket, meta.Key, err)
	}
	if versionID := firstVisibleVersionID(versions); versionID != "" {
		return versionID, nil
	}
	return fallbackVersionID(meta), nil
}

func firstVisibleVersionID(versions *collectionlist.List[model.ObjectVersion]) string {
	if versions == nil {
		return ""
	}
	version, found := collectionlist.FindList(versions, func(_ int, version model.ObjectVersion) bool {
		return !version.DeleteMarker && strings.TrimSpace(version.VersionID) != ""
	})
	if !found {
		return ""
	}
	return strings.TrimSpace(version.VersionID)
}

func fallbackVersionID(meta model.ObjectMeta) string {
	candidate, found := collectionlist.FindList(collectionlist.NewList(meta.ETag, meta.Hash), func(_ int, candidate string) bool {
		return strings.TrimSpace(candidate) != ""
	})
	if found {
		return strings.TrimSpace(candidate)
	}
	if !meta.UpdatedAt.IsZero() {
		return meta.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return "current"
}

func (manager *Manager) recordIndexDocument(
	ctx context.Context,
	meta model.ObjectMeta,
	versionID string,
	state string,
	indexedAt time.Time,
	cause error,
) error {
	document := model.IndexDocument{
		Bucket:    meta.Bucket,
		Key:       meta.Key,
		VersionID: versionID,
		Digest:    meta.Hash,
		State:     state,
		IndexedAt: indexedAt,
	}
	if cause != nil {
		document.Error = failureMessage(cause)
	}
	if _, err := manager.metadata.UpsertIndexDocument(ctx, document); err != nil {
		return fmt.Errorf("upsert index document %s/%s: %w", meta.Bucket, meta.Key, err)
	}
	return nil
}

func (manager *Manager) pruneObjects(objects *collectionlist.List[model.ObjectMeta], result *RebuildResult) error {
	if err := manager.search.PruneExcept(objects); err != nil {
		result.Failed++
		return fmt.Errorf("prune stale search index documents: %w", err)
	}
	return nil
}

func (manager *Manager) markRebuildJob(
	ctx context.Context,
	id string,
	status string,
	result RebuildResult,
	message string,
) error {
	job := model.IndexJob{
		ID:          id,
		Kind:        model.IndexJobKindRebuild,
		Status:      status,
		Attempts:    1,
		Error:       truncateMessage(message),
		AvailableAt: result.StartedAt,
		StartedAt:   result.StartedAt,
		FinishedAt:  result.FinishedAt,
	}
	if _, err := manager.metadata.UpsertIndexJob(ctx, job); err != nil {
		return fmt.Errorf("upsert rebuild index job: %w", err)
	}
	return nil
}

func rebuildJobStatus(result RebuildResult, err error, message string) string {
	if err != nil || message != "" || result.Failed > 0 {
		return model.IndexJobStatusFailed
	}
	return model.IndexJobStatusSucceeded
}

func rebuildJobID(startedAt time.Time) string {
	return "rebuild:" + startedAt.UTC().Format("20060102T150405.000000000Z")
}

func firstErrorMessage(current string, err error) string {
	if err == nil {
		return current
	}
	return firstMessage(current, failureMessage(err))
}

func firstMessage(current, message string) string {
	if current != "" {
		return current
	}
	return truncateMessage(message)
}

func failureMessage(cause error) string {
	if cause == nil {
		return ""
	}
	return truncateMessage(cause.Error())
}

func truncateMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > maxIndexManagerErrorLength {
		return message[:maxIndexManagerErrorLength]
	}
	return message
}
