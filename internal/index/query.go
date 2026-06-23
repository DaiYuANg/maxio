package index

import (
	"github.com/blevesearch/bleve/v2"
	qry "github.com/blevesearch/bleve/v2/search/query"
	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/lo"
	"strings"
)

func (s *SearchEngine) buildQuery(criteria model.SearchQuery) qry.Query {
	queries := compactQueries(
		textQuery(criteria.Query),
		fieldMatchQuery("bucket", strings.ToLower(criteria.Bucket)),
		fieldMatchQuery("content_type", strings.ToLower(criteria.ContentType)),
		prefixQuery("key", criteria.Prefix),
		textQuery(criteria.NameContains),
		sizeRangeQuery(criteria),
	)

	if len(queries) == 0 {
		return bleve.NewMatchAllQuery()
	}
	if len(queries) == 1 {
		return queries[0]
	}
	return bleve.NewConjunctionQuery(queries...)
}

func compactQueries(queries ...qry.Query) []qry.Query {
	return lo.Filter(queries, func(query qry.Query, _ int) bool {
		return query != nil
	})
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
	if criteria.MinSize <= 0 && criteria.MaxSize <= 0 {
		return nil
	}
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
	bucket := normalizedSearchField(query.Bucket)
	return (bucket == "" || normalizedSearchField(meta.Bucket) == bucket) &&
		(query.Prefix == "" || strings.HasPrefix(meta.Key, query.Prefix))
}

func matchesMetadata(meta model.ObjectMeta, query model.SearchQuery) bool {
	contentType := normalizedSearchField(query.ContentType)
	return (query.NameContains == "" || strings.Contains(meta.Key, query.NameContains)) &&
		(contentType == "" || normalizedSearchField(meta.ContentType) == contentType)
}

func matchesSize(meta model.ObjectMeta, query model.SearchQuery) bool {
	return (query.MinSize <= 0 || meta.Size >= query.MinSize) &&
		(query.MaxSize <= 0 || meta.Size <= query.MaxSize)
}

func normalizedSearchField(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
