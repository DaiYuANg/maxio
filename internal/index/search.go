package index

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/arcgolabs/collectionx/list"
	collectionmapping "github.com/arcgolabs/collectionx/mapping"
	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const indexDir = "index/bleve"

type SearchEngine struct {
	logger *slog.Logger
	index  bleve.Index
	ready  bool
	mu     sync.RWMutex
	items  map[string]*model.ObjectMeta
}

type IndexDocument struct {
	Meta model.ObjectMeta
	Text string
}

func NewSearchEngine(cfg Config, logger *slog.Logger) (*SearchEngine, error) {
	if logger == nil {
		logger = slog.Default()
	}
	idx, err := openPersistentIndex(cfg)
	if err != nil {
		return nil, err
	}
	return &SearchEngine{
		logger: logger,
		index:  idx,
		ready:  true,
		items:  make(map[string]*model.ObjectMeta),
	}, nil
}

func NewInMemorySearchEngine() *SearchEngine {
	mapping := bleve.NewIndexMapping()
	idx, err := bleve.NewMemOnly(mapping)
	if err != nil {
		slog.Default().Error("search index init failed", "error", err)
		return &SearchEngine{
			logger: slog.Default(),
			items:  make(map[string]*model.ObjectMeta),
		}
	}
	return &SearchEngine{
		logger: slog.Default(),
		index:  idx,
		ready:  true,
		items:  make(map[string]*model.ObjectMeta),
	}
}

func openPersistentIndex(cfg Config) (bleve.Index, error) {
	cfg = cfg.normalized()
	path := filepath.Join(cfg.DataDir, indexDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create search index parent: %w", err)
	}
	idx, err := bleve.Open(path)
	if err == nil {
		return idx, nil
	}
	if !errors.Is(err, bleve.ErrorIndexPathDoesNotExist) {
		return nil, fmt.Errorf("open search index: %w", err)
	}
	idx, err = bleve.New(path, bleve.NewIndexMapping())
	if err != nil {
		return nil, fmt.Errorf("create search index: %w", err)
	}
	return idx, nil
}

func (s *SearchEngine) Upsert(meta model.ObjectMeta) {
	if _, err := s.UpsertDocuments([]IndexDocument{{Meta: meta}}); err != nil {
		s.logger.Warn("upsert search index failed", "error", err)
	}
}

func (s *SearchEngine) UpsertDocument(meta model.ObjectMeta, text string) {
	if _, err := s.UpsertDocuments([]IndexDocument{{Meta: meta, Text: text}}); err != nil {
		s.logger.Warn("upsert search index failed", "error", err)
	}
}

func (s *SearchEngine) UpsertDocuments(docs []IndexDocument) (int, error) {
	if s == nil || len(docs) == 0 {
		return 0, nil
	}
	if !s.ready {
		s.upsertMemoryDocuments(docs)
		return len(docs), nil
	}

	batch := s.index.NewBatch()
	for i := range docs {
		doc := docs[i]
		id := objectID(doc.Meta.Bucket, doc.Meta.Key)
		if err := batch.Index(id, documentFromMeta(doc.Meta, doc.Text)); err != nil {
			return 0, fmt.Errorf("prepare search index batch: %w", err)
		}
	}

	if err := s.index.Batch(batch); err != nil {
		s.logger.Warn("upsert search index batch failed", "error", err)
		return s.indexSingleDocuments(docs, err)
	}

	s.upsertMemoryDocuments(docs)
	return len(docs), nil
}

func (s *SearchEngine) indexSingleDocuments(docs []IndexDocument, batchErr error) (int, error) {
	successes := 0
	for i := range docs {
		doc := docs[i]
		if doc.Meta.Bucket == "" || doc.Meta.Key == "" {
			continue
		}
		id := objectID(doc.Meta.Bucket, doc.Meta.Key)
		if err := s.index.Index(id, documentFromMeta(doc.Meta, doc.Text)); err != nil {
			batchErr = errors.Join(batchErr, fmt.Errorf("upsert search document %s: %w", id, err))
			continue
		}
		successes++
	}
	s.upsertMemoryDocuments(docs)
	if batchErr != nil {
		return successes, batchErr
	}
	return successes, nil
}

func (s *SearchEngine) upsertMemoryDocuments(docs []IndexDocument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range docs {
		doc := docs[i]
		if doc.Meta.Bucket == "" || doc.Meta.Key == "" {
			continue
		}
		meta := doc.Meta
		s.items[objectID(doc.Meta.Bucket, doc.Meta.Key)] = &meta
	}
}

func (s *SearchEngine) Remove(bucket, key string) {
	id := objectID(bucket, key)
	if s.ready {
		if err := s.index.Delete(id); err != nil {
			s.logger.Warn("remove search index failed", "error", err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, id)
}

func (s *SearchEngine) Search(query model.SearchQuery) model.SearchResult {
	if !s.ready {
		return s.searchFromMemory(query)
	}

	hits, err := s.searchIndex(query)
	if err != nil {
		s.logger.Warn("search index query failed", "error", err)
		return s.searchFromMemory(query)
	}
	return s.resultFromHits(query, hits)
}

func (s *SearchEngine) Close() error {
	if s == nil || s.index == nil {
		return nil
	}
	if err := s.index.Close(); err != nil {
		return fmt.Errorf("close search index: %w", err)
	}
	return nil
}

func (s *SearchEngine) searchIndex(query model.SearchQuery) (*list.List[searchHit], error) {
	req := bleve.NewSearchRequest(s.buildQuery(query))
	if query.Limit > 0 {
		req.Size = query.Limit
	}
	req.Fields = []string{"bucket", "key", "hash", "etag", "size", "content_type", "updated_at"}
	result, err := s.index.Search(req)
	if err != nil {
		return nil, fmt.Errorf("search bleve index: %w", err)
	}
	hits := list.MapList(list.NewList(result.Hits...), func(_ int, hit *search.DocumentMatch) searchHit {
		return searchHit{
			ID:     hit.ID,
			Fields: hit.Fields,
		}
	})
	return hits, nil
}

type searchHit struct {
	ID     string
	Fields map[string]any
}

func (s *SearchEngine) resultFromHits(query model.SearchQuery, hits *list.List[searchHit]) model.SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := list.FilterMapList(hits, func(_ int, hit searchHit) (model.ObjectMeta, bool) {
		if meta, ok := s.items[hit.ID]; ok && meta != nil {
			return *meta, true
		}
		fromIndex := metaFromFields(hit.Fields)
		if fromIndex.Bucket == "" || fromIndex.Key == "" {
			return model.ObjectMeta{}, false
		}
		return fromIndex, true
	})
	return limitedSearchResult(query, items)
}

func (s *SearchEngine) searchFromMemory(query model.SearchQuery) model.SearchResult {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := list.FilterMapList(
		listValuesFromMap(s.items),
		func(_ int, meta *model.ObjectMeta) (model.ObjectMeta, bool) {
			if meta == nil {
				return model.ObjectMeta{}, false
			}
			if !matchesQuery(*meta, query) {
				return model.ObjectMeta{}, false
			}
			return *meta, true
		},
	)
	return limitedSearchResult(query, items)
}

func listValuesFromMap[K comparable, V any](values map[K]V) *list.List[V] {
	return list.NewList(collectionmapping.NewMapFrom(values).Values()...)
}

func listKeysFromMap[K comparable, V any](values map[K]V) *list.List[K] {
	return list.NewList(collectionmapping.NewMapFrom(values).Keys()...)
}
