package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	dbsm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/lyonbrown4d/maxio/model"
)

const (
	MetadataErrorBadRequest     = "bad_request"
	MetadataErrorBucketExists   = "bucket_exists"
	MetadataErrorBucketNotFound = "bucket_not_found"
	MetadataErrorObjectNotFound = "object_not_found"

	MetadataCommandCreateBucket            = "create_bucket"
	MetadataCommandDeleteBucket            = "delete_bucket"
	MetadataCommandStageObjectMeta         = "stage_object_meta"
	MetadataCommandUpsertObjectMeta        = "upsert_object_meta"
	MetadataCommandDeleteStagedObjectMeta  = "delete_staged_object_meta"
	MetadataCommandDeleteObjectMeta        = "delete_object_meta"
	MetadataCommandCreateBlobRef           = "create_blob_ref"
	MetadataCommandUpdateBlobRefPlacements = "update_blob_ref_placements"
	MetadataCommandIncreaseBlobRef         = "increase_blob_ref"
	MetadataCommandDecreaseBlobRef         = "decrease_blob_ref"

	MetadataQueryListBuckets           = "list_buckets"
	MetadataQueryBucketExists          = "bucket_exists"
	MetadataQueryListObjectMetas       = "list_object_metas"
	MetadataQueryListStagedObjectMetas = "list_staged_object_metas"
	MetadataQueryListBlobRefs          = "list_blob_refs"
	MetadataQueryGetObjectMeta         = "get_object_meta"
	MetadataQueryGetBlobRef            = "get_blob_ref"
)

type MetadataBlobRef struct {
	Hash            string                 `json:"hash,omitempty"`
	Path            string                 `json:"path"`
	ShardPlacements []model.ShardPlacement `json:"shard_placements,omitempty"`
	ShardChecksums  []string               `json:"shard_checksums,omitempty"`
	ShardSizes      []int64                `json:"shard_sizes,omitempty"`
	RefCount        int                    `json:"ref_count"`
	Size            int64                  `json:"size"`
}

type MetadataCommand struct {
	Type            string                 `json:"type"`
	Bucket          string                 `json:"bucket,omitempty"`
	Key             string                 `json:"key,omitempty"`
	BucketMeta      model.Bucket           `json:"bucket_meta"`
	Meta            model.ObjectMeta       `json:"meta"`
	Hash            string                 `json:"hash,omitempty"`
	Path            string                 `json:"path,omitempty"`
	Size            int64                  `json:"size,omitempty"`
	ShardPlacements []model.ShardPlacement `json:"shard_placements,omitempty"`
	ShardChecksums  []string               `json:"shard_checksums,omitempty"`
	ShardSizes      []int64                `json:"shard_sizes,omitempty"`
}

type MetadataQuery struct {
	Type   string `json:"type"`
	Bucket string `json:"bucket,omitempty"`
	Key    string `json:"key,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Hash   string `json:"hash,omitempty"`
}

type MetadataResult struct {
	Buckets      []model.Bucket     `json:"buckets,omitempty"`
	BucketExists bool               `json:"bucket_exists,omitempty"`
	Objects      []model.ObjectMeta `json:"objects,omitempty"`
	Object       model.ObjectMeta   `json:"object"`
	ObjectExists bool               `json:"object_exists,omitempty"`
	Blobs        []MetadataBlobRef  `json:"blobs,omitempty"`
	Blob         MetadataBlobRef    `json:"blob"`
	BlobExists   bool               `json:"blob_exists,omitempty"`
	BlobPath     string             `json:"blob_path,omitempty"`
	BlobRemoved  bool               `json:"blob_removed,omitempty"`
}

type metadataEnvelope struct {
	Result MetadataResult `json:"result"`
	Error  string         `json:"error,omitempty"`
}

type metadataSnapshot struct {
	Buckets       map[string]model.Bucket     `json:"buckets"`
	Objects       map[string]model.ObjectMeta `json:"objects"`
	StagedObjects map[string]model.ObjectMeta `json:"staged_objects"`
	BlobRefs      map[string]MetadataBlobRef  `json:"blob_refs"`
}

type raftStateMachine struct {
	shardID   uint64
	replicaID uint64

	mu            sync.RWMutex
	closed        bool
	buckets       map[string]model.Bucket
	objects       map[string]model.ObjectMeta
	stagedObjects map[string]model.ObjectMeta
	blobRefs      map[string]MetadataBlobRef
}

func newRaftStateMachine(shardID, replicaID uint64) *raftStateMachine {
	return &raftStateMachine{
		shardID:       shardID,
		replicaID:     replicaID,
		buckets:       make(map[string]model.Bucket),
		objects:       make(map[string]model.ObjectMeta),
		stagedObjects: make(map[string]model.ObjectMeta),
		blobRefs:      make(map[string]MetadataBlobRef),
	}
}

func (s *raftStateMachine) Lookup(query any) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return metadataEnvelope{Error: MetadataErrorBadRequest}, nil
	}

	switch typed := query.(type) {
	case MetadataQuery:
		return s.lookupMetadataQuery(typed), nil
	case *MetadataQuery:
		if typed == nil {
			return metadataEnvelope{Error: MetadataErrorBadRequest}, nil
		}
		return s.lookupMetadataQuery(*typed), nil
	case string:
		switch typed {
		case "health":
			return metadataEnvelope{}, nil
		case "state":
			return metadataEnvelope{Result: MetadataResult{BucketExists: true}}, nil
		default:
			return metadataEnvelope{Error: MetadataErrorBadRequest}, nil
		}
	default:
		return metadataEnvelope{Error: MetadataErrorBadRequest}, nil
	}
}

func (s *raftStateMachine) Update(entry dbsm.Entry) (dbsm.Result, error) {
	var cmd MetadataCommand
	if err := json.Unmarshal(entry.Cmd, &cmd); err != nil {
		return encodeMetadataEnvelope(metadataEnvelope{Error: MetadataErrorBadRequest})
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return encodeMetadataEnvelope(metadataEnvelope{Error: MetadataErrorBadRequest})
	}

	result, code := s.applyMetadataCommand(cmd)
	return encodeMetadataEnvelope(metadataEnvelope{Result: result, Error: code})
}

func (s *raftStateMachine) SaveSnapshot(w io.Writer, _ dbsm.ISnapshotFileCollection, _ <-chan struct{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := json.NewEncoder(w).Encode(metadataSnapshot{
		Buckets:       copyBucketMap(s.buckets),
		Objects:       copyObjectMap(s.objects),
		StagedObjects: copyObjectMap(s.stagedObjects),
		BlobRefs:      copyBlobRefMap(s.blobRefs),
	}); err != nil {
		return fmt.Errorf("encode metadata snapshot: %w", err)
	}
	return nil
}

func (s *raftStateMachine) RecoverFromSnapshot(r io.Reader, _ []dbsm.SnapshotFile, _ <-chan struct{}) error {
	var snapshot metadataSnapshot
	if err := json.NewDecoder(r).Decode(&snapshot); err != nil {
		return fmt.Errorf("decode metadata snapshot: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.buckets = snapshot.Buckets
	if s.buckets == nil {
		s.buckets = make(map[string]model.Bucket)
	}
	s.objects = snapshot.Objects
	if s.objects == nil {
		s.objects = make(map[string]model.ObjectMeta)
	}
	s.stagedObjects = snapshot.StagedObjects
	if s.stagedObjects == nil {
		s.stagedObjects = make(map[string]model.ObjectMeta)
	}
	s.blobRefs = snapshot.BlobRefs
	if s.blobRefs == nil {
		s.blobRefs = make(map[string]MetadataBlobRef)
	}
	return nil
}

func (s *raftStateMachine) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	return nil
}
