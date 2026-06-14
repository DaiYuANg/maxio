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
	ref, found, err := m.store.GetDigestRef(ctx, digest)
	if err != nil {
		return model.DigestRef{}, false, oops.Wrapf(err, "get digest ref")
	}
	if !found {
		return model.DigestRef{}, false, nil
	}
	if strings.TrimSpace(ref.UpstreamID) != strings.TrimSpace(upstreamID) {
		return model.DigestRef{}, false, nil
	}
	if strings.TrimSpace(ref.UpstreamBucket) == "" || strings.TrimSpace(ref.UpstreamKey) == "" {
		return model.DigestRef{}, false, nil
	}
	return ref, true, nil
}

func (m *dedupeMiddleware) commitObjectPut(ctx context.Context, event ObjectPutSucceededEvent) (model.ObjectVersion, error) {
	if err := m.ensureBucket(ctx, event.Bucket); err != nil {
		return model.ObjectVersion{}, err
	}
	previousDigest, hasPrevious, err := m.currentDigest(ctx, event.Bucket, event.Key)
	if err != nil {
		return model.ObjectVersion{}, oops.Wrapf(err, "load current object digest")
	}
	if _, retainErr := m.store.RetainDigestRef(ctx, model.DigestRef{
		Digest:         event.Digest,
		Size:           event.Size,
		RefCount:       1,
		UpstreamID:     event.UpstreamID,
		UpstreamBucket: event.UpstreamBucket,
		UpstreamKey:    event.UpstreamKey,
	}); retainErr != nil {
		return model.ObjectVersion{}, oops.Wrapf(retainErr, "retain digest ref")
	}
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
	if hasPrevious {
		if err := m.releaseDigest(ctx, previousDigest); err != nil {
			return model.ObjectVersion{}, oops.Wrapf(err, "release previous digest")
		}
	}
	return version, nil
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
	return version, true, nil
}

func (m *dedupeMiddleware) releaseDigest(ctx context.Context, digest string) error {
	if strings.TrimSpace(digest) == "" {
		return nil
	}
	if _, _, err := m.store.ReleaseDigestRef(ctx, digest); err != nil && !errors.Is(err, metadata.ErrObjectNotFound) {
		return oops.Wrapf(err, "release digest ref")
	}
	return nil
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
