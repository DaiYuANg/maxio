package metadata

import (
	"github.com/arcgolabs/dbx"
	repositoryx "github.com/arcgolabs/dbx/repository"
	"github.com/lyonbrown4d/maxio/internal/model"
)

type metadataSQLRepositories struct {
	buckets         *repositoryx.Base[model.Bucket, metadataBucketsSchema]
	upstreams       *repositoryx.Base[model.Upstream, metadataUpstreamsSchema]
	blobRefs        *repositoryx.Base[BlobRef, metadataBlobRefsSchema]
	objects         *repositoryx.Base[model.ObjectMeta, metadataObjectsSchema]
	objectRecords   *repositoryx.Base[model.ObjectRecord, metadataObjectRecordsSchema]
	objectVersions  *repositoryx.Base[model.ObjectVersion, metadataObjectVersionsSchema]
	digestRefs      *repositoryx.Base[model.DigestRef, metadataDigestRefsSchema]
	indexDocuments  *repositoryx.Base[model.IndexDocument, metadataIndexDocumentsSchema]
	indexJobs       *repositoryx.Base[model.IndexJob, metadataIndexJobsSchema]
	indexOutbox     *repositoryx.Base[model.IndexOutboxEvent, metadataIndexOutboxSchema]
	schedulerLeases *repositoryx.Base[metadataSchedulerLease, metadataSchedulerLeasesSchema]
}

func newMetadataSQLRepositories(db *dbx.DB) metadataSQLRepositories {
	if db == nil {
		return metadataSQLRepositories{}
	}
	return metadataSQLRepositories{
		buckets:         repositoryx.New[model.Bucket](db, metadataBuckets.schema),
		upstreams:       repositoryx.New[model.Upstream](db, metadataUpstreams.schema),
		blobRefs:        repositoryx.New[BlobRef](db, metadataBlobRefs.schema),
		objects:         repositoryx.New[model.ObjectMeta](db, metadataObjects.schema),
		objectRecords:   repositoryx.New[model.ObjectRecord](db, metadataObjectRecords.schema),
		objectVersions:  repositoryx.New[model.ObjectVersion](db, metadataObjectVersions.schema),
		digestRefs:      repositoryx.New[model.DigestRef](db, metadataDigestRefs.schema),
		indexDocuments:  repositoryx.New[model.IndexDocument](db, metadataIndexDocuments.schema),
		indexJobs:       repositoryx.New[model.IndexJob](db, metadataIndexJobs.schema),
		indexOutbox:     repositoryx.New[model.IndexOutboxEvent](db, metadataIndexOutbox.schema),
		schedulerLeases: repositoryx.New[metadataSchedulerLease](db, metadataSchedulerLeases.schema),
	}
}
