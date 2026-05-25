package raft

import (
	"sort"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

func (s *raftStateMachine) applyMetadataCommand(cmd MetadataCommand) (MetadataResult, string) {
	if code, ok := s.applyBucketCommand(cmd); ok {
		return MetadataResult{}, code
	}
	if result, code, ok := s.applyObjectCommand(cmd); ok {
		return result, code
	}
	if result, code, ok := s.applyBlobCommand(cmd); ok {
		return result, code
	}
	return MetadataResult{}, MetadataErrorBadRequest
}

func (s *raftStateMachine) applyBucketCommand(cmd MetadataCommand) (string, bool) {
	switch cmd.Type {
	case MetadataCommandCreateBucket:
		return s.createBucket(cmd.Bucket, cmd.BucketMeta), true
	case MetadataCommandDeleteBucket:
		return s.deleteBucket(cmd.Bucket), true
	default:
		return "", false
	}
}

func (s *raftStateMachine) applyObjectCommand(cmd MetadataCommand) (MetadataResult, string, bool) {
	switch cmd.Type {
	case MetadataCommandStageObjectMeta:
		return MetadataResult{}, s.stageObjectMeta(cmd.Meta), true
	case MetadataCommandUpsertObjectMeta:
		return MetadataResult{}, s.upsertObjectMeta(cmd.Meta), true
	case MetadataCommandDeleteStagedObjectMeta:
		result, code := s.deleteStagedObjectMeta(cmd.Bucket, cmd.Key)
		return result, code, true
	case MetadataCommandDeleteObjectMeta:
		result, code := s.deleteObjectMeta(cmd.Bucket, cmd.Key)
		return result, code, true
	default:
		return MetadataResult{}, "", false
	}
}

func (s *raftStateMachine) applyBlobCommand(cmd MetadataCommand) (MetadataResult, string, bool) {
	switch cmd.Type {
	case MetadataCommandCreateBlobRef:
		return MetadataResult{}, s.createBlobRef(cmd.Hash, cmd.Path, cmd.Size, cmd.ShardPlacements, cmd.ShardChecksums, cmd.ShardSizes), true
	case MetadataCommandUpdateBlobRefPlacements:
		return MetadataResult{}, s.updateBlobRefPlacements(cmd.Hash, cmd.ShardPlacements), true
	case MetadataCommandIncreaseBlobRef:
		return MetadataResult{}, s.increaseBlobRef(cmd.Hash), true
	case MetadataCommandDecreaseBlobRef:
		result, code := s.decreaseBlobRef(cmd.Hash)
		return result, code, true
	default:
		return MetadataResult{}, "", false
	}
}

func (s *raftStateMachine) lookupMetadataQuery(query MetadataQuery) metadataEnvelope {
	switch query.Type {
	case MetadataQueryListBuckets:
		return metadataEnvelope{Result: MetadataResult{Buckets: s.listBuckets()}}
	case MetadataQueryBucketExists:
		return s.lookupBucketExists(query.Bucket)
	case MetadataQueryListObjectMetas:
		return s.lookupListObjectMetas(query.Bucket, query.Prefix)
	case MetadataQueryListStagedObjectMetas:
		return s.lookupListStagedObjectMetas(query.Bucket, query.Prefix)
	case MetadataQueryListBlobRefs:
		return metadataEnvelope{Result: MetadataResult{Blobs: s.listBlobRefs()}}
	case MetadataQueryGetObjectMeta:
		return s.lookupObjectMeta(query.Bucket, query.Key)
	case MetadataQueryGetBlobRef:
		return s.lookupBlobRef(query.Hash)
	default:
		return metadataEnvelope{Error: MetadataErrorBadRequest}
	}
}

func (s *raftStateMachine) lookupBucketExists(bucket string) metadataEnvelope {
	if invalidName(bucket) {
		return metadataEnvelope{Error: MetadataErrorBadRequest}
	}
	_, ok := s.buckets[bucket]
	return metadataEnvelope{Result: MetadataResult{BucketExists: ok}}
}

func (s *raftStateMachine) lookupListObjectMetas(bucket, prefix string) metadataEnvelope {
	if _, ok := s.buckets[bucket]; !ok {
		return metadataEnvelope{Error: MetadataErrorBucketNotFound}
	}
	return metadataEnvelope{Result: MetadataResult{Objects: s.listObjectMetas(bucket, prefix)}}
}

func (s *raftStateMachine) lookupObjectMeta(bucket, key string) metadataEnvelope {
	if invalidObject(bucket, key) {
		return metadataEnvelope{Error: MetadataErrorBadRequest}
	}
	meta, ok := s.objects[objectMapKey(bucket, key)]
	return metadataEnvelope{Result: MetadataResult{Object: meta, ObjectExists: ok}}
}

func (s *raftStateMachine) lookupBlobRef(hash string) metadataEnvelope {
	if invalidName(hash) {
		return metadataEnvelope{Error: MetadataErrorBadRequest}
	}
	ref, ok := s.blobRefs[hash]
	ref.Hash = hash
	return metadataEnvelope{Result: MetadataResult{Blob: ref, BlobExists: ok}}
}

func (s *raftStateMachine) createBucket(bucket string, bucketMeta model.Bucket) string {
	if bucketMeta.Name != "" {
		bucket = bucketMeta.Name
	}
	if invalidName(bucket) {
		return MetadataErrorBadRequest
	}
	if _, ok := s.buckets[bucket]; ok {
		return MetadataErrorBucketExists
	}
	if bucketMeta.Name == "" {
		bucketMeta.Name = bucket
	}
	s.buckets[bucket] = bucketMeta
	return ""
}

func (s *raftStateMachine) deleteBucket(bucket string) string {
	if invalidName(bucket) {
		return MetadataErrorBadRequest
	}
	if _, ok := s.buckets[bucket]; !ok {
		return MetadataErrorBucketNotFound
	}
	delete(s.buckets, bucket)
	for key := range s.objects {
		meta := s.objects[key]
		if meta.Bucket == bucket {
			delete(s.objects, key)
		}
	}
	for key := range s.stagedObjects {
		meta := s.stagedObjects[key]
		if meta.Bucket == bucket {
			delete(s.stagedObjects, key)
		}
	}
	return ""
}

func (s *raftStateMachine) upsertObjectMeta(meta model.ObjectMeta) string {
	if invalidObject(meta.Bucket, meta.Key) || invalidName(meta.Hash) {
		return MetadataErrorBadRequest
	}
	if _, ok := s.buckets[meta.Bucket]; !ok {
		return MetadataErrorBucketNotFound
	}
	meta.State = model.ObjectStateCommitted
	s.objects[objectMapKey(meta.Bucket, meta.Key)] = meta
	return ""
}

func (s *raftStateMachine) stageObjectMeta(meta model.ObjectMeta) string {
	if invalidObject(meta.Bucket, meta.Key) || invalidName(meta.Hash) {
		return MetadataErrorBadRequest
	}
	if _, ok := s.buckets[meta.Bucket]; !ok {
		return MetadataErrorBucketNotFound
	}
	meta.State = model.ObjectStatePending
	s.stagedObjects[objectMapKey(meta.Bucket, meta.Key)] = meta
	return ""
}

func (s *raftStateMachine) deleteStagedObjectMeta(bucket, key string) (MetadataResult, string) {
	if invalidObject(bucket, key) {
		return MetadataResult{}, MetadataErrorBadRequest
	}
	mapKey := objectMapKey(bucket, key)
	meta, ok := s.stagedObjects[mapKey]
	if !ok {
		return MetadataResult{ObjectExists: false}, ""
	}
	delete(s.stagedObjects, mapKey)
	return MetadataResult{Object: meta, ObjectExists: true}, ""
}

func (s *raftStateMachine) deleteObjectMeta(bucket, key string) (MetadataResult, string) {
	if invalidObject(bucket, key) {
		return MetadataResult{}, MetadataErrorBadRequest
	}
	mapKey := objectMapKey(bucket, key)
	meta, ok := s.objects[mapKey]
	if !ok {
		return MetadataResult{ObjectExists: false}, ""
	}
	delete(s.objects, mapKey)
	return MetadataResult{Object: meta, ObjectExists: true}, ""
}

func (s *raftStateMachine) listBuckets() []model.Bucket {
	buckets := make([]model.Bucket, 0, len(s.buckets))
	for _, bucket := range s.buckets {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Name < buckets[j].Name
	})
	return buckets
}

func (s *raftStateMachine) listObjectMetas(bucket, prefix string) []model.ObjectMeta {
	objects := make([]model.ObjectMeta, 0)
	for key := range s.objects {
		meta := s.objects[key]
		if meta.Bucket == bucket && strings.HasPrefix(meta.Key, prefix) {
			objects = append(objects, meta)
		}
	}
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})
	return objects
}
