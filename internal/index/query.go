package index

import (
	"strings"

	"github.com/blevesearch/bleve/v2"
	qry "github.com/blevesearch/bleve/v2/search/query"
	"github.com/lyonbrown4d/maxio/internal/model"
)

func (s *SearchEngine) buildQuery(criteria model.SearchQuery) qry.Query {
	queries := make([]qry.Query, 0, 6)
	queries = appendQuery(queries, textQuery(criteria.Query))
	queries = appendQuery(queries, fieldMatchQuery("bucket", strings.ToLower(criteria.Bucket)))
	queries = appendQuery(queries, fieldMatchQuery("content_type", strings.ToLower(criteria.ContentType)))
	queries = appendQuery(queries, prefixQuery("key", criteria.Prefix))
	queries = appendQuery(queries, textQuery(criteria.NameContains))
	if criteria.MinSize > 0 || criteria.MaxSize > 0 {
		queries = append(queries, sizeRangeQuery(criteria))
	}

	if len(queries) == 0 {
		return bleve.NewMatchAllQuery()
	}
	if len(queries) == 1 {
		return queries[0]
	}
	return bleve.NewConjunctionQuery(queries...)
}

func appendQuery(queries []qry.Query, query qry.Query) []qry.Query {
	if query == nil {
		return queries
	}
	return append(queries, query)
}

func textQuery(value string) qry.Query {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	query := bleve.NewMatchQuery(value)
	query.SetField("text")
	return query
}

func fieldMatchQuery(field, value string) qry.Query {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	query := bleve.NewMatchQuery(value)
	query.SetField(field)
	return query
}

func prefixQuery(field, value string) qry.Query {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	query := bleve.NewPrefixQuery(value)
	query.SetField(field)
	return query
}

func sizeRangeQuery(criteria model.SearchQuery) qry.Query {
	var minSize, maxSize *float64
	if criteria.MinSize > 0 {
		minSizeValue := float64(criteria.MinSize)
		minSize = &minSizeValue
	}
	if criteria.MaxSize > 0 {
		maxSizeValue := float64(criteria.MaxSize)
		maxSize = &maxSizeValue
	}
	size := bleve.NewNumericRangeQuery(minSize, maxSize)
	size.SetField("size")
	return size
}

func matchesQuery(meta model.ObjectMeta, query model.SearchQuery) bool {
	return matchesLocation(meta, query) && matchesMetadata(meta, query) && matchesSize(meta, query)
}

func matchesLocation(meta model.ObjectMeta, query model.SearchQuery) bool {
	return (query.Bucket == "" || meta.Bucket == query.Bucket) &&
		(query.Prefix == "" || strings.HasPrefix(meta.Key, query.Prefix))
}

func matchesMetadata(meta model.ObjectMeta, query model.SearchQuery) bool {
	return (query.NameContains == "" || strings.Contains(meta.Key, query.NameContains)) &&
		(query.ContentType == "" || meta.ContentType == query.ContentType)
}

func matchesSize(meta model.ObjectMeta, query model.SearchQuery) bool {
	return (query.MinSize <= 0 || meta.Size >= query.MinSize) &&
		(query.MaxSize <= 0 || meta.Size <= query.MaxSize)
}
