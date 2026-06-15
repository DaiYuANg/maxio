// Package metadata provides metadata persistence abstractions and implementations
// for object/bucket/index state used by maxio.
package metadata

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/arcgolabs/collectionx/list"
	collectionset "github.com/arcgolabs/collectionx/set"
	"github.com/lyonbrown4d/maxio/internal/model"
)

var (
	ErrBadRequest     = errors.New("bad request")
	ErrBucketExists   = errors.New("bucket already exists")
	ErrBucketNotFound = errors.New("bucket not found")
	ErrObjectNotFound = errors.New("object not found")
	ErrUnsupported    = errors.New("unsupported metadata operation")
)

type BlobRef struct {
	Hash     string
	Path     string
	RefCount int
	Size     int64
}

type Repository interface {
	ListUpstreams(ctx context.Context) (*list.List[model.Upstream], error)
	GetUpstream(ctx context.Context, id string) (model.Upstream, bool, error)
	UpsertUpstream(ctx context.Context, upstream model.Upstream) (model.Upstream, error)
	DeleteUpstream(ctx context.Context, id string) (bool, error)

	ListBuckets(ctx context.Context) (*list.List[model.Bucket], error)
	BucketExists(ctx context.Context, bucket string) (bool, error)
	CreateBucket(ctx context.Context, bucket string) error
	DeleteBucket(ctx context.Context, bucket string) error

	ListObjectMetas(ctx context.Context, bucket, prefix string) (*list.List[model.ObjectMeta], error)
	ListStagedObjectMetas(ctx context.Context, bucket, prefix string) (*list.List[model.ObjectMeta], error)
	GetObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error)
	StageObjectMeta(ctx context.Context, meta model.ObjectMeta) error
	UpsertObjectMeta(ctx context.Context, meta model.ObjectMeta) error
	DeleteStagedObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error)
	DeleteObjectMeta(ctx context.Context, bucket, key string) (model.ObjectMeta, bool, error)

	UpsertObjectRecord(ctx context.Context, record model.ObjectRecord) (model.ObjectRecord, error)
	GetObjectRecord(ctx context.Context, bucket, key string) (model.ObjectRecord, bool, error)
	DeleteObjectRecord(ctx context.Context, bucket, key string) (bool, error)
	UpsertObjectVersion(ctx context.Context, version model.ObjectVersion) (model.ObjectVersion, error)
	GetObjectVersion(ctx context.Context, bucket, key, versionID string) (model.ObjectVersion, bool, error)
	ListObjectVersions(ctx context.Context, bucket, key string) (*list.List[model.ObjectVersion], error)
	DeleteObjectVersion(ctx context.Context, bucket, key, versionID string) (bool, error)
	UpsertDigestRef(ctx context.Context, ref model.DigestRef) (model.DigestRef, error)
	GetDigestRef(ctx context.Context, digest string) (model.DigestRef, bool, error)
	RetainDigestRef(ctx context.Context, ref model.DigestRef) (model.DigestRef, error)
	ReleaseDigestRef(ctx context.Context, digest string) (model.DigestRef, bool, error)
	DeleteDigestRef(ctx context.Context, digest string) (bool, error)
	UpsertIndexDocument(ctx context.Context, document model.IndexDocument) (model.IndexDocument, error)
	GetIndexDocument(ctx context.Context, id string) (model.IndexDocument, bool, error)
	ListIndexDocuments(ctx context.Context, bucket, prefix string) (*list.List[model.IndexDocument], error)
	DeleteIndexDocument(ctx context.Context, id string) (bool, error)
	UpsertIndexJob(ctx context.Context, job model.IndexJob) (model.IndexJob, error)
	GetIndexJob(ctx context.Context, id string) (model.IndexJob, bool, error)
	ListIndexJobs(ctx context.Context, status string, limit int) (*list.List[model.IndexJob], error)
	DeleteIndexJob(ctx context.Context, id string) (bool, error)
	UpsertIndexOutboxEvent(ctx context.Context, event model.IndexOutboxEvent) (model.IndexOutboxEvent, error)
	GetIndexOutboxEvent(ctx context.Context, id string) (model.IndexOutboxEvent, bool, error)
	ListIndexOutboxEvents(ctx context.Context, status string, limit int) (*list.List[model.IndexOutboxEvent], error)
	DeleteIndexOutboxEvent(ctx context.Context, id string) (bool, error)

	ListBlobRefs(ctx context.Context) (*list.List[BlobRef], error)
	GetBlobRef(ctx context.Context, hash string) (BlobRef, bool, error)
	CreateBlobRef(ctx context.Context, hash, path string, size int64) error
	IncreaseBlobRef(ctx context.Context, hash string) error
	DecreaseBlobRef(ctx context.Context, hash string) (string, bool, error)
}

type MetadataStore = Repository

type InMemoryMetadata struct {
	mu        sync.RWMutex
	buckets   map[string]*collectionset.Set[string]
	upstreams map[string]model.Upstream
	objects   map[string]model.ObjectMeta
	staged    map[string]model.ObjectMeta
	blobs     map[string]BlobRef

	objectRecords  map[string]model.ObjectRecord
	objectVersions map[string]model.ObjectVersion
	digestRefs     map[string]model.DigestRef
	indexDocuments map[string]model.IndexDocument
	indexJobs      map[string]model.IndexJob
	indexOutbox    map[string]model.IndexOutboxEvent
}

func NewInMemoryMetadata() *InMemoryMetadata {
	return &InMemoryMetadata{
		buckets:   make(map[string]*collectionset.Set[string]),
		upstreams: make(map[string]model.Upstream),
		objects:   make(map[string]model.ObjectMeta),
		staged:    make(map[string]model.ObjectMeta),
		blobs:     make(map[string]BlobRef),

		objectRecords:  make(map[string]model.ObjectRecord),
		objectVersions: make(map[string]model.ObjectVersion),
		digestRefs:     make(map[string]model.DigestRef),
		indexDocuments: make(map[string]model.IndexDocument),
		indexJobs:      make(map[string]model.IndexJob),
		indexOutbox:    make(map[string]model.IndexOutboxEvent),
	}
}

func (m *InMemoryMetadata) ListBuckets(context.Context) (*list.List[model.Bucket], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now().UTC()
	buckets := list.NewListWithCapacity[model.Bucket](len(m.buckets))
	for name := range m.buckets {
		buckets.Add(model.Bucket{
			Name:      name,
			CreatedAt: now,
		})
	}
	sorted := buckets.Sort(func(left, right model.Bucket) int {
		if left.Name < right.Name {
			return -1
		}
		if left.Name > right.Name {
			return 1
		}
		return 0
	})
	return &sorted, nil
}

func (m *InMemoryMetadata) BucketExists(_ context.Context, bucket string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return false, ErrBadRequest
	}
	_, ok := m.buckets[bucket]
	return ok, nil
}

func (m *InMemoryMetadata) CreateBucket(_ context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.buckets[bucket]; ok {
		return ErrBucketExists
	}
	m.buckets[bucket] = collectionset.NewSet[string]()
	return nil
}

func (m *InMemoryMetadata) DeleteBucket(_ context.Context, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	keys, ok := m.buckets[bucket]
	if !ok {
		return ErrBucketNotFound
	}
	for _, key := range keys.Values() {
		id := objectID(bucket, key)
		meta := m.objects[id]
		delete(m.objects, id)
		if _, _, err := m.decreaseBlobRefLocked(meta.Hash); err != nil && !errors.Is(err, ErrObjectNotFound) {
			return err
		}
	}
	delete(m.buckets, bucket)
	for key := range m.staged {
		meta := m.staged[key]
		if meta.Bucket == bucket {
			delete(m.staged, key)
		}
	}
	return nil
}

func (m *InMemoryMetadata) ListObjectMetas(_ context.Context, bucket, prefix string) (*list.List[model.ObjectMeta], error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, ErrBadRequest
	}
	keys, ok := m.buckets[bucket]
	if !ok {
		return nil, ErrBucketNotFound
	}

	result := list.NewListWithCapacity[model.ObjectMeta](keys.Len())
	for _, key := range keys.Values() {
		if strings.HasPrefix(key, prefix) {
			meta := m.objects[objectID(bucket, key)]
			result.Add(meta)
		}
	}
	sorted := result.Sort(func(left, right model.ObjectMeta) int {
		if left.Key < right.Key {
			return -1
		}
		if left.Key > right.Key {
			return 1
		}
		return 0
	})
	return &sorted, nil
}

func (m *InMemoryMetadata) GetObjectMeta(_ context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectMeta{}, false, ErrBadRequest
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	meta, ok := m.objects[objectID(bucket, key)]
	if !ok {
		return model.ObjectMeta{}, false, nil
	}
	return meta, true, nil
}

func (m *InMemoryMetadata) UpsertObjectMeta(_ context.Context, meta model.ObjectMeta) error {
	meta.Bucket = strings.TrimSpace(meta.Bucket)
	meta.Key = strings.TrimSpace(meta.Key)
	if meta.Bucket == "" || meta.Key == "" {
		return ErrBadRequest
	}
	meta.State = model.ObjectStateCommitted

	m.mu.Lock()
	defer m.mu.Unlock()
	keys, ok := m.buckets[meta.Bucket]
	if !ok {
		return ErrBucketNotFound
	}
	keys.Add(meta.Key)
	m.objects[objectID(meta.Bucket, meta.Key)] = meta
	return nil
}

func (m *InMemoryMetadata) DeleteObjectMeta(_ context.Context, bucket, key string) (model.ObjectMeta, bool, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return model.ObjectMeta{}, false, ErrBadRequest
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	id := objectID(bucket, key)
	meta, ok := m.objects[id]
	if !ok {
		return model.ObjectMeta{}, false, nil
	}
	delete(m.objects, id)
	if keys, ok := m.buckets[bucket]; ok {
		keys.Remove(key)
	}
	return meta, true, nil
}

func objectID(bucket, key string) string {
	return bucket + "\x00" + key
}
