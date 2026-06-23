package index

import (
	"fmt"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/collectionx/set"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const pruneSearchPageSize = 1000

// PruneExcept removes indexed documents that are no longer present in the
// committed object metadata snapshot.
func (s *SearchEngine) PruneExcept(valid *collectionlist.List[model.ObjectMeta]) error {
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

func objectIDSet(objects *collectionlist.List[model.ObjectMeta]) *set.Set[string] {
	if objects == nil || objects.Len() == 0 {
		return set.NewSet[string]()
	}
	ids := collectionlist.FilterMapList(objects, func(_ int, meta model.ObjectMeta) (string, bool) {
		if meta.Bucket == "" || meta.Key == "" {
			return "", false
		}
		return objectID(meta.Bucket, meta.Key), true
	})
	return set.NewSetWithCapacity[string](ids.Len(), ids.Values()...)
}

func (s *SearchEngine) indexedDocumentIDs() (*set.Set[string], error) {
	ids := s.memoryDocumentIDs()
	if !s.ready {
		return ids, nil
	}
	bleveIDs, err := s.bleveDocumentIDs()
	if err != nil {
		return nil, err
	}
	ids.MergeSlice(bleveIDs.Values())
	return ids, nil
}

func (s *SearchEngine) memoryDocumentIDs() *set.Set[string] {
	s.mu.RLock()
	defer s.mu.RUnlock()

	itemIDs := listKeysFromMap(s.items)
	return set.NewSetWithCapacity[string](itemIDs.Len(), itemIDs.Values()...)
}

func (s *SearchEngine) deleteStaleDocuments(indexedIDs, validIDs *set.Set[string]) (*collectionlist.List[string], error) {
	staleSet := indexedIDs.Difference(validIDs)
	staleIDs := collectionlist.NewListWithCapacity[string](staleSet.Len())
	var deleteErr error
	staleSet.Range(func(id string) bool {
		staleIDs.Add(id)
		if err := s.deleteStaleDocument(id); err != nil {
			deleteErr = err
			return false
		}
		return true
	})
	if deleteErr != nil {
		return nil, deleteErr
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

func (s *SearchEngine) removeMemoryDocuments(ids *collectionlist.List[string]) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids.Range(func(_ int, id string) bool {
		delete(s.items, id)
		return true
	})
}

func (s *SearchEngine) bleveDocumentIDs() (*collectionlist.List[string], error) {
	ids := collectionlist.NewList[string]()
	for offset := 0; ; offset += pruneSearchPageSize {
		req := bleve.NewSearchRequest(bleve.NewMatchAllQuery())
		req.Size = pruneSearchPageSize
		req.From = offset
		result, err := s.index.Search(req)
		if err != nil {
			return nil, fmt.Errorf("search indexed document ids: %w", err)
		}
		if len(result.Hits) > 0 {
			hitIDs := collectionlist.FilterMapList(result.Hits, func(_ int, hit *search.DocumentMatch) (string, bool) {
				if hit == nil {
					return "", false
				}
				return hit.ID, true
			})
			ids.MergeSlice(hitIDs.Values())
		}
		if len(result.Hits) < pruneSearchPageSize {
			break
		}
	}
	return ids, nil
}
