package metadata

import (
	"context"

	"github.com/lyonbrown4d/maxio/model"
)

func (r *RaftMetadata) UpsertObjectRecord(context.Context, model.ObjectRecord) (model.ObjectRecord, error) {
	return model.ObjectRecord{}, ErrUnsupported
}

func (r *RaftMetadata) GetObjectRecord(context.Context, string, string) (model.ObjectRecord, bool, error) {
	return model.ObjectRecord{}, false, ErrUnsupported
}

func (r *RaftMetadata) DeleteObjectRecord(context.Context, string, string) (bool, error) {
	return false, ErrUnsupported
}

func (r *RaftMetadata) UpsertObjectVersion(context.Context, model.ObjectVersion) (model.ObjectVersion, error) {
	return model.ObjectVersion{}, ErrUnsupported
}

func (r *RaftMetadata) GetObjectVersion(context.Context, string, string, string) (model.ObjectVersion, bool, error) {
	return model.ObjectVersion{}, false, ErrUnsupported
}

func (r *RaftMetadata) ListObjectVersions(context.Context, string, string) ([]model.ObjectVersion, error) {
	return nil, ErrUnsupported
}

func (r *RaftMetadata) DeleteObjectVersion(context.Context, string, string, string) (bool, error) {
	return false, ErrUnsupported
}

func (r *RaftMetadata) UpsertDigestRef(context.Context, model.DigestRef) (model.DigestRef, error) {
	return model.DigestRef{}, ErrUnsupported
}

func (r *RaftMetadata) GetDigestRef(context.Context, string) (model.DigestRef, bool, error) {
	return model.DigestRef{}, false, ErrUnsupported
}

func (r *RaftMetadata) RetainDigestRef(context.Context, model.DigestRef) (model.DigestRef, error) {
	return model.DigestRef{}, ErrUnsupported
}

func (r *RaftMetadata) ReleaseDigestRef(context.Context, string) (model.DigestRef, bool, error) {
	return model.DigestRef{}, false, ErrUnsupported
}

func (r *RaftMetadata) DeleteDigestRef(context.Context, string) (bool, error) {
	return false, ErrUnsupported
}

func (r *RaftMetadata) UpsertIndexDocument(context.Context, model.IndexDocument) (model.IndexDocument, error) {
	return model.IndexDocument{}, ErrUnsupported
}

func (r *RaftMetadata) GetIndexDocument(context.Context, string) (model.IndexDocument, bool, error) {
	return model.IndexDocument{}, false, ErrUnsupported
}

func (r *RaftMetadata) ListIndexDocuments(context.Context, string, string) ([]model.IndexDocument, error) {
	return nil, ErrUnsupported
}

func (r *RaftMetadata) DeleteIndexDocument(context.Context, string) (bool, error) {
	return false, ErrUnsupported
}

func (r *RaftMetadata) UpsertIndexJob(context.Context, model.IndexJob) (model.IndexJob, error) {
	return model.IndexJob{}, ErrUnsupported
}

func (r *RaftMetadata) GetIndexJob(context.Context, string) (model.IndexJob, bool, error) {
	return model.IndexJob{}, false, ErrUnsupported
}

func (r *RaftMetadata) ListIndexJobs(context.Context, string, int) ([]model.IndexJob, error) {
	return nil, ErrUnsupported
}

func (r *RaftMetadata) DeleteIndexJob(context.Context, string) (bool, error) {
	return false, ErrUnsupported
}

func (r *RaftMetadata) UpsertIndexOutboxEvent(context.Context, model.IndexOutboxEvent) (model.IndexOutboxEvent, error) {
	return model.IndexOutboxEvent{}, ErrUnsupported
}

func (r *RaftMetadata) GetIndexOutboxEvent(context.Context, string) (model.IndexOutboxEvent, bool, error) {
	return model.IndexOutboxEvent{}, false, ErrUnsupported
}

func (r *RaftMetadata) ListIndexOutboxEvents(context.Context, string, int) ([]model.IndexOutboxEvent, error) {
	return nil, ErrUnsupported
}

func (r *RaftMetadata) DeleteIndexOutboxEvent(context.Context, string) (bool, error) {
	return false, ErrUnsupported
}
