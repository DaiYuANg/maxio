package index

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/arcgolabs/collectionx/list"
	"github.com/lyonbrown4d/maxio/internal/model"
)

type document struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	Hash        string `json:"hash"`
	ETag        string `json:"etag"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	UpdatedAt   string `json:"updated_at"`
	Text        string `json:"text"`
}

func limitedSearchResult(query model.SearchQuery, items *list.List[model.ObjectMeta]) model.SearchResult {
	sorted := items.Sort(func(left, right model.ObjectMeta) int {
		switch {
		case left.UpdatedAt.After(right.UpdatedAt):
			return -1
		case left.UpdatedAt.Before(right.UpdatedAt):
			return 1
		default:
			return 0
		}
	})
	resultItems := sorted.Values()
	if query.Limit > 0 && len(resultItems) > query.Limit {
		resultItems = resultItems[:query.Limit]
	}
	return model.SearchResult{Items: resultItems}
}

func documentFromMeta(meta model.ObjectMeta, text string) document {
	return document{
		Bucket:      strings.ToLower(meta.Bucket),
		Key:         meta.Key,
		Hash:        meta.Hash,
		ETag:        meta.ETag,
		Size:        meta.Size,
		ContentType: strings.ToLower(meta.ContentType),
		UpdatedAt:   meta.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Text:        meta.Key + "\n" + text,
	}
}

func metaFromFields(fields map[string]any) model.ObjectMeta {
	return model.ObjectMeta{
		Bucket:      stringField(fields, "bucket"),
		Key:         stringField(fields, "key"),
		Hash:        stringField(fields, "hash"),
		ETag:        stringField(fields, "etag"),
		Size:        int64Field(fields, "size"),
		ContentType: stringField(fields, "content_type"),
		UpdatedAt:   timeField(fields, "updated_at"),
		State:       model.ObjectStateCommitted,
	}
}

func stringField(fields map[string]any, name string) string {
	value, ok := fields[name]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func int64Field(fields map[string]any, name string) int64 {
	value, ok := fields[name]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case string:
		return parseInt64Field(typed)
	default:
		return 0
	}
}

func parseInt64Field(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func timeField(fields map[string]any, name string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, stringField(fields, name))
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func objectID(bucket, key string) string {
	return bucket + "\x00" + key
}
