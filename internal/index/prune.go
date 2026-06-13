package index

import (
	"fmt"

	"github.com/blevesearch/bleve/v2"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const pruneSearchPageSize = 1000

// PruneExcept removes indexed documents that are no longer present in the
// committed object metadata snapshot.
func (s *SearchEngine) PruneExcept(valid []model.ObjectMeta) error {
	if s == nil {
		return nil
	}

	indexedIDs, err := s.indexedDocumentIDs()
	if err != nil {
		return err
	}
	staleIDs, err := s.deleteStaleDocuments(indexedIDs, objectIDSet(valid))
	if err != nil {
		return err
	}
	s.removeMemoryDocuments(staleIDs)
	return nil
}

func objectIDSet(objects []model.ObjectMeta) map[string]struct{} {
	ids := make(map[string]struct{}, len(objects))
	for index := range objects {
		meta := objects[index]
		if meta.Bucket == "" || meta.Key == "" {
			continue
		}
		ids[objectID(meta.Bucket, meta.Key)] = struct{}{}
	}
	return ids
}

func (s *SearchEngine) indexedDocumentIDs() (map[string]struct{}, error) {
	ids := s.memoryDocumentIDs()
	if !s.ready {
		return ids, nil
	}
	bleveIDs, err := s.bleveDocumentIDs()
	if err != nil {
		return nil, err
	}
	for _, id := range bleveIDs {
		ids[id] = struct{}{}
	}
	return ids, nil
}

func (s *SearchEngine) memoryDocumentIDs() map[string]struct{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make(map[string]struct{}, len(s.items))
	for id := range s.items {
		ids[id] = struct{}{}
	}
	return ids
}

func (s *SearchEngine) deleteStaleDocuments(indexedIDs, validIDs map[string]struct{}) ([]string, error) {
	staleIDs := make([]string, 0)
	for id := range indexedIDs {
		if _, ok := validIDs[id]; ok {
			continue
		}
		if err := s.deleteStaleDocument(id); err != nil {
			return nil, err
		}
		staleIDs = append(staleIDs, id)
	}
	return staleIDs, nil
}

func (s *SearchEngine) deleteStaleDocument(id string) error {
	if !s.ready {
		return nil
	}
	if err := s.index.Delete(id); err != nil {
		return fmt.Errorf("delete stale search index document %q: %w", id, err)
	}
	return nil
}

func (s *SearchEngine) removeMemoryDocuments(ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range ids {
		delete(s.items, id)
	}
}

func (s *SearchEngine) bleveDocumentIDs() ([]string, error) {
	ids := make([]string, 0)
	for offset := 0; ; offset += pruneSearchPageSize {
		req := bleve.NewSearchRequest(bleve.NewMatchAllQuery())
		req.Size = pruneSearchPageSize
		req.From = offset
		result, err := s.index.Search(req)
		if err != nil {
			return nil, fmt.Errorf("search indexed document ids: %w", err)
		}
		for _, hit := range result.Hits {
			ids = append(ids, hit.ID)
		}
		if len(result.Hits) < pruneSearchPageSize {
			break
		}
	}
	return ids, nil
}
