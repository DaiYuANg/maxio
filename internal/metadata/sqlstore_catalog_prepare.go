package metadata

import (
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func prepareDBObjectRecord(record model.ObjectRecord) (model.ObjectRecord, error) {
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

func prepareDBObjectVersion(version model.ObjectVersion) (model.ObjectVersion, error) {
	version.Bucket, version.Key, version.VersionID = trimObjectVersionKey(version.Bucket, version.Key, version.VersionID)
	version.Digest = strings.TrimSpace(version.Digest)
	version.ETag = strings.TrimSpace(version.ETag)
	version.UpstreamID = strings.TrimSpace(version.UpstreamID)
	version.UpstreamBucket = strings.TrimSpace(version.UpstreamBucket)
	version.UpstreamKey = strings.TrimSpace(version.UpstreamKey)
	if version.Bucket == "" || version.Key == "" || version.VersionID == "" || (!version.DeleteMarker && version.Digest == "") {
		return model.ObjectVersion{}, ErrBadRequest
	}
	now := time.Now().UTC()
	if version.CreatedAt.IsZero() {
		version.CreatedAt = now
	}
	version.UpdatedAt = now
	return version, nil
}

func prepareDBDigestRef(ref model.DigestRef) (model.DigestRef, error) {
	ref.Digest = strings.TrimSpace(ref.Digest)
	ref.UpstreamID = strings.TrimSpace(ref.UpstreamID)
	ref.UpstreamBucket = strings.TrimSpace(ref.UpstreamBucket)
	ref.UpstreamKey = strings.TrimSpace(ref.UpstreamKey)
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

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intToBool(value int) bool {
	return value != 0
}
