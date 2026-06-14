package proxy

import (
	"context"
	"errors"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/oops"
)

func (m *dedupeMiddleware) reusableDigestRef(ctx context.Context, digest, upstreamID string) (model.DigestRef, bool, error) {
	if _, ok := m.cachedReusableDigestRef(ctx, digest, upstreamID); ok {
		confirmed, found, err := m.store.GetDigestRef(ctx, digest)
		if err != nil {
			return model.DigestRef{}, false, oops.Wrapf(err, "confirm cached digest ref")
		}
		if found {
			m.cacheDigestRef(ctx, confirmed)
			return confirmed, sameReusableUpstream(confirmed, upstreamID), nil
		}
		m.deleteCachedDigestRef(ctx, digest)
		return model.DigestRef{}, false, nil
	}
	ref, found, err := m.store.GetDigestRef(ctx, digest)
	if err != nil {
		return model.DigestRef{}, false, oops.Wrapf(err, "get digest ref")
	}
	if !found {
		return model.DigestRef{}, false, nil
	}
	m.cacheDigestRef(ctx, ref)
	return ref, sameReusableUpstream(ref, upstreamID), nil
}

func (m *dedupeMiddleware) commitObjectPut(ctx context.Context, event ObjectPutSucceededEvent) (model.ObjectVersion, error) {
	if err := m.ensureBucket(ctx, event.Bucket); err != nil {
		return model.ObjectVersion{}, err
	}
	previousDigest, hasPrevious, err := m.currentDigest(ctx, event.Bucket, event.Key)
	if err != nil {
		return model.ObjectVersion{}, oops.Wrapf(err, "load current object digest")
	}
	retainedRef, err := m.retainObjectDigest(ctx, event)
	if err != nil {
		return model.ObjectVersion{}, err
	}
	version, err := m.writeObjectVersionAndRecord(ctx, event)
	if err != nil {
		return model.ObjectVersion{}, err
	}
	retainedRef, err = m.releasePreviousDigestIfNeeded(ctx, previousDigest, hasPrevious, event.Digest, retainedRef)
	if err != nil {
		return model.ObjectVersion{}, err
	}
	m.cacheDigestRef(ctx, retainedRef)
	m.cacheObjectVersion(ctx, version)
	return version, nil
}

func (m *dedupeMiddleware) retainObjectDigest(ctx context.Context, event ObjectPutSucceededEvent) (model.DigestRef, error) {
	ref, err := m.store.RetainDigestRef(ctx, model.DigestRef{
		Digest:         event.Digest,
		Size:           event.Size,
		RefCount:       1,
		UpstreamID:     event.UpstreamID,
		UpstreamBucket: event.UpstreamBucket,
		UpstreamKey:    event.UpstreamKey,
	})
	if err != nil {
		return model.DigestRef{}, oops.Wrapf(err, "retain digest ref")
	}
	return ref, nil
}

func (m *dedupeMiddleware) writeObjectVersionAndRecord(
	ctx context.Context,
	event ObjectPutSucceededEvent,
) (model.ObjectVersion, error) {
	version, err := m.store.UpsertObjectVersion(ctx, model.ObjectVersion{
		Bucket:         event.Bucket,
		Key:            event.Key,
		VersionID:      event.VersionID,
		Digest:         event.Digest,
		ETag:           strings.TrimSpace(event.ETag),
		Size:           event.Size,
		ContentType:    strings.TrimSpace(event.ContentType),
		UserMetadata:   event.UserMetadata,
		UpstreamID:     event.UpstreamID,
		UpstreamBucket: event.UpstreamBucket,
		UpstreamKey:    event.UpstreamKey,
	})
	if err != nil {
		return model.ObjectVersion{}, oops.Wrapf(err, "upsert object version")
	}
	if _, err := m.store.UpsertObjectRecord(ctx, model.ObjectRecord{
		Bucket:           event.Bucket,
		Key:              event.Key,
		CurrentVersionID: version.VersionID,
		Deleted:          false,
	}); err != nil {
		return model.ObjectVersion{}, oops.Wrapf(err, "upsert object record")
	}
	return version, nil
}

func (m *dedupeMiddleware) releasePreviousDigestIfNeeded(
	ctx context.Context,
	previousDigest string,
	hasPrevious bool,
	currentDigest string,
	retainedRef model.DigestRef,
) (model.DigestRef, error) {
	if !hasPrevious {
		return retainedRef, nil
	}
	if err := m.releaseDigest(ctx, previousDigest); err != nil {
		return model.DigestRef{}, oops.Wrapf(err, "release previous digest")
	}
	if previousDigest != currentDigest {
		return retainedRef, nil
	}
	currentRef, found, err := m.store.GetDigestRef(ctx, currentDigest)
	if err != nil {
		return model.DigestRef{}, oops.Wrapf(err, "reload retained digest ref")
	}
	if found {
		return currentRef, nil
	}
	return retainedRef, nil
}

func (m *dedupeMiddleware) ensureBucket(ctx context.Context, bucket string) error {
	exists, err := m.store.BucketExists(ctx, bucket)
	if err != nil {
		return oops.Wrapf(err, "check bucket exists")
	}
	if exists {
		return nil
	}
	if err := m.store.CreateBucket(ctx, bucket); err != nil && !errors.Is(err, metadata.ErrBucketExists) {
		return oops.Wrapf(err, "create bucket metadata")
	}
	return nil
}

func (m *dedupeMiddleware) currentDigest(ctx context.Context, bucket, key string) (string, bool, error) {
	version, found, err := m.currentObjectVersion(ctx, bucket, key)
	if err != nil {
		return "", false, err
	}
	if !found || version.Digest == "" {
		return "", false, nil
	}
	return version.Digest, true, nil
}

func (m *dedupeMiddleware) currentObjectVersion(ctx context.Context, bucket, key string) (model.ObjectVersion, bool, error) {
	if version, found := m.cachedObjectVersion(ctx, bucket, key); found {
		return version, true, nil
	}
	return m.storeCurrentObjectVersion(ctx, bucket, key)
}

func (m *dedupeMiddleware) cachedObjectVersion(ctx context.Context, bucket, key string) (model.ObjectVersion, bool) {
	if m.cache == nil {
		return model.ObjectVersion{}, false
	}
	version, found, err := m.cache.GetObjectVersion(ctx, bucket, key)
	if err == nil && found {
		return version, true
	}
	if err != nil && m.logger != nil {
		m.logger.WarnContext(ctx, "get cached object version", "bucket", bucket, "key", key, "error", err)
	}
	return model.ObjectVersion{}, false
}

func (m *dedupeMiddleware) storeCurrentObjectVersion(ctx context.Context, bucket, key string) (model.ObjectVersion, bool, error) {
	record, ok, err := m.store.GetObjectRecord(ctx, bucket, key)
	if err != nil {
		return model.ObjectVersion{}, false, oops.Wrapf(err, "get object record")
	}
	if !ok || record.CurrentVersionID == "" {
		return model.ObjectVersion{}, false, nil
	}
	version, ok, err := m.store.GetObjectVersion(ctx, bucket, key, record.CurrentVersionID)
	if err != nil {
		return model.ObjectVersion{}, false, oops.Wrapf(err, "get object version")
	}
	if !ok {
		return model.ObjectVersion{}, false, nil
	}
	m.cacheObjectVersion(ctx, version)
	return version, true, nil
}

func (m *dedupeMiddleware) releaseDigest(ctx context.Context, digest string) error {
	if strings.TrimSpace(digest) == "" {
		return nil
	}
	ref, removed, err := m.store.ReleaseDigestRef(ctx, digest)
	if err != nil && !errors.Is(err, metadata.ErrObjectNotFound) {
		return oops.Wrapf(err, "release digest ref")
	}
	if removed || errors.Is(err, metadata.ErrObjectNotFound) {
		m.deleteCachedDigestRef(ctx, digest)
		return nil
	}
	m.cacheDigestRef(ctx, ref)
	return nil
}

func sameReusableUpstream(ref model.DigestRef, upstreamID string) bool {
	if strings.TrimSpace(ref.UpstreamID) != strings.TrimSpace(upstreamID) {
		return false
	}
	return strings.TrimSpace(ref.UpstreamBucket) != "" && strings.TrimSpace(ref.UpstreamKey) != ""
}

func (m *dedupeMiddleware) cachedReusableDigestRef(ctx context.Context, digest, upstreamID string) (model.DigestRef, bool) {
	if m.cache == nil {
		return model.DigestRef{}, false
	}
	ref, found, err := m.cache.GetDigestRef(ctx, digest)
	if err != nil {
		if m.logger != nil {
			m.logger.WarnContext(ctx, "get cached digest ref", "digest", digest, "error", err)
		}
		return model.DigestRef{}, false
	}
	return ref, found && sameReusableUpstream(ref, upstreamID)
}

func (m *dedupeMiddleware) cacheObjectVersion(ctx context.Context, version model.ObjectVersion) {
	if m.cache == nil {
		return
	}
	if err := m.cache.SetObjectVersion(ctx, version); err != nil && m.logger != nil {
		m.logger.WarnContext(ctx, "set cached object version", "bucket", version.Bucket, "key", version.Key, "error", err)
	}
}

func (m *dedupeMiddleware) deleteCachedObjectVersion(ctx context.Context, bucket, key string) {
	if m.cache == nil {
		return
	}
	if err := m.cache.DeleteObjectVersion(ctx, bucket, key); err != nil && m.logger != nil {
		m.logger.WarnContext(ctx, "delete cached object version", "bucket", bucket, "key", key, "error", err)
	}
}

func (m *dedupeMiddleware) cacheDigestRef(ctx context.Context, ref model.DigestRef) {
	if m.cache == nil {
		return
	}
	if err := m.cache.SetDigestRef(ctx, ref); err != nil && m.logger != nil {
		m.logger.WarnContext(ctx, "set cached digest ref", "digest", ref.Digest, "error", err)
	}
}

func (m *dedupeMiddleware) deleteCachedDigestRef(ctx context.Context, digest string) {
	if m.cache == nil {
		return
	}
	if err := m.cache.DeleteDigestRef(ctx, digest); err != nil && m.logger != nil {
		m.logger.WarnContext(ctx, "delete cached digest ref", "digest", digest, "error", err)
	}
}

func (m *dedupeMiddleware) publishObjectPut(ctx context.Context, event ObjectPutSucceededEvent) {
	if m.bus == nil {
		return
	}
	eventCtx := context.WithoutCancel(ctx)
	if err := m.bus.PublishAsync(eventCtx, event); err != nil {
		m.logger.ErrorContext(ctx, "publish s3 proxy object put event", "bucket", event.Bucket, "key", event.Key, "error", err)
	}
}

func (m *dedupeMiddleware) publishObjectDelete(ctx context.Context, event ObjectDeleteSucceededEvent) {
	if m.bus == nil {
		return
	}
	eventCtx := context.WithoutCancel(ctx)
	if err := m.bus.PublishAsync(eventCtx, event); err != nil {
		m.logger.ErrorContext(ctx, "publish s3 proxy object delete event", "bucket", event.Bucket, "key", event.Key, "error", err)
	}
}
