package metadata

import (
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/model"
)

func prepareMemoryObjectRecord(record model.ObjectRecord) (model.ObjectRecord, error) {
	record.Bucket = strings.TrimSpace(record.Bucket)
	record.Key = strings.TrimSpace(record.Key)
	record.CurrentVersionID = strings.TrimSpace(record.CurrentVersionID)
	if record.Bucket == "" || record.Key == "" {
		return model.ObjectRecord{}, ErrBadRequest
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	return record, nil
}

func prepareMemoryObjectVersion(version model.ObjectVersion) (model.ObjectVersion, error) {
	version.Bucket, version.Key, version.VersionID = trimObjectVersionKey(version.Bucket, version.Key, version.VersionID)
	version.Digest = strings.TrimSpace(version.Digest)
	if version.Bucket == "" || version.Key == "" || version.VersionID == "" || (!version.DeleteMarker && version.Digest == "") {
		return model.ObjectVersion{}, ErrBadRequest
	}
	now := time.Now().UTC()
	if version.CreatedAt.IsZero() {
		version.CreatedAt = now
	}
	version.UpdatedAt = now
	return cloneObjectVersion(version), nil
}

func prepareMemoryDigestRef(ref model.DigestRef) (model.DigestRef, error) {
	ref.Digest = strings.TrimSpace(ref.Digest)
	if ref.Digest == "" || ref.Size < 0 {
		return model.DigestRef{}, ErrBadRequest
	}
	if ref.RefCount <= 0 {
		ref.RefCount = 1
	}
	now := time.Now().UTC()
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = now
	}
	ref.UpdatedAt = now
	return ref, nil
}

func cloneObjectRecord(record model.ObjectRecord) model.ObjectRecord {
	return record
}

func cloneObjectVersion(version model.ObjectVersion) model.ObjectVersion {
	version.UserMetadata = cloneStringMap(version.UserMetadata)
	return version
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func trimObjectVersionKey(bucket, key, versionID string) (string, string, string) {
	return strings.TrimSpace(bucket), strings.TrimSpace(key), strings.TrimSpace(versionID)
}

func objectVersionID(bucket, key, versionID string) string {
	return objectID(bucket, key) + "\x00" + versionID
}

func indexEntityID(bucket, key, versionID string) string {
	bucket, key, versionID = trimObjectVersionKey(bucket, key, versionID)
	if bucket == "" || key == "" || versionID == "" {
		return ""
	}
	return objectVersionID(bucket, key, versionID)
}

func normalizeListLimit(limit int) int {
	if limit <= 0 {
		return defaultMetadataListLimit
	}
	return limit
}
