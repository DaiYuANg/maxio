package raft

import (
	"sort"
	"strings"

	"github.com/lyonbrown4d/maxio/model"
)

func (s *raftStateMachine) lookupListStagedObjectMetas(bucket, prefix string) metadataEnvelope {
	if bucket != "" {
		if _, ok := s.buckets[bucket]; !ok {
			return metadataEnvelope{Error: MetadataErrorBucketNotFound}
		}
	}
	return metadataEnvelope{Result: MetadataResult{Objects: s.listStagedObjectMetas(bucket, prefix)}}
}

func (s *raftStateMachine) listStagedObjectMetas(bucket, prefix string) []model.ObjectMeta {
	objects := make([]model.ObjectMeta, 0)
	for key := range s.stagedObjects {
		meta := s.stagedObjects[key]
		if matchesStagedObject(meta, bucket, prefix) {
			objects = append(objects, meta)
		}
	}
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].Bucket == objects[j].Bucket {
			return objects[i].Key < objects[j].Key
		}
		return objects[i].Bucket < objects[j].Bucket
	})
	return objects
}

func matchesStagedObject(meta model.ObjectMeta, bucket, prefix string) bool {
	if bucket != "" && meta.Bucket != bucket {
		return false
	}
	return prefix == "" || strings.HasPrefix(meta.Key, prefix)
}
