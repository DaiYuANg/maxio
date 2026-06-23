package index

import (
	"context"
	"fmt"
	"strings"
	"time"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (manager *Manager) listCommittedObjectMetas(ctx context.Context) (*collectionlist.List[model.ObjectMeta], error) {
	buckets, err := manager.metadata.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	objects := collectionlist.NewList[model.ObjectMeta]()
	if buckets == nil {
		return objects, nil
	}
	var listErr error
	buckets.Range(func(_ int, bucket model.Bucket) bool {
		metas, err := manager.metadata.ListObjectMetas(ctx, bucket.Name, "")
		if err != nil {
			listErr = fmt.Errorf("list %q object metadata: %w", bucket.Name, err)
			return false
		}
		if metas != nil {
			objects.MergeSlice(metas.Values())
		}
		return true
	})
	if listErr != nil {
		return nil, listErr
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
		if err := ctx.Err(); err != nil {
			message = firstErrorMessage(message, err)
			return false
		}
		versionID, err := manager.currentVersionID(ctx, meta)
		if err != nil {
			failed++
			message = firstErrorMessage(message, err)
			return true
		}
		count, err := manager.search.UpsertDocuments([]IndexDocument{{Meta: meta}})
		if err != nil || count == 0 {
			failed++
			if err == nil {
				err = fmt.Errorf("search index accepted no documents for %s/%s", meta.Bucket, meta.Key)
			}
			message = firstErrorMessage(message, err)
			if recordErr := manager.recordIndexDocument(ctx, meta, versionID, model.IndexDocumentStateFailed, time.Time{}, err); recordErr != nil {
				message = firstErrorMessage(message, recordErr)
			}
			return true
		}
		if err := manager.recordIndexDocument(ctx, meta, versionID, model.IndexDocumentStateIndexed, now(), nil); err != nil {
			failed++
			message = firstErrorMessage(message, err)
			return true
		}
		indexed++
		return true
	})
	return indexed, failed, message
}

func (manager *Manager) currentVersionID(ctx context.Context, meta model.ObjectMeta) (string, error) {
	record, found, err := manager.metadata.GetObjectRecord(ctx, meta.Bucket, meta.Key)
	if err != nil {
		return "", fmt.Errorf("get object record %s/%s: %w", meta.Bucket, meta.Key, err)
	}
	if found && strings.TrimSpace(record.CurrentVersionID) != "" {
		return strings.TrimSpace(record.CurrentVersionID), nil
	}

	versions, err := manager.metadata.ListObjectVersions(ctx, meta.Bucket, meta.Key)
	if err != nil {
		return "", fmt.Errorf("list object versions %s/%s: %w", meta.Bucket, meta.Key, err)
	}
	if versions != nil {
		versionID := ""
		versions.Range(func(_ int, version model.ObjectVersion) bool {
			if version.DeleteMarker || strings.TrimSpace(version.VersionID) == "" {
				return true
			}
			versionID = strings.TrimSpace(version.VersionID)
			return false
		})
		if versionID != "" {
			return versionID, nil
		}
	}
	return fallbackVersionID(meta), nil
}

func fallbackVersionID(meta model.ObjectMeta) string {
	for _, candidate := range []string{meta.ETag, meta.Hash} {
		if value := strings.TrimSpace(candidate); value != "" {
			return value
		}
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

func truncateMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > maxIndexJobErrorLength {
		return message[:maxIndexJobErrorLength]
	}
	return message
}
