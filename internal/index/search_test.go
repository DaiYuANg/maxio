package index_test

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/model"
)

func TestSearchEnginePersistsFullTextIndex(t *testing.T) {
	t.Parallel()

	cfg := config.Config{DataDir: t.TempDir()}
	engine := newSearchEngine(t, cfg)
	meta := model.ObjectMeta{
		Bucket:      "docs",
		Key:         "guide.txt",
		Hash:        "hash",
		ETag:        `"etag"`,
		Size:        42,
		ContentType: "text/plain",
		UpdatedAt:   time.Now().UTC(),
		State:       model.ObjectStateCommitted,
	}
	engine.UpsertDocument(meta, "hello searchable maxio content")
	mustCloseSearchEngine(t, engine)

	reopened := newSearchEngine(t, cfg)
	defer mustCloseSearchEngine(t, reopened)
	result := reopened.Search(model.SearchQuery{Query: "searchable"})
	if len(result.Items) != 1 {
		t.Fatalf("search hits = %d, want 1", len(result.Items))
	}
	if result.Items[0].Bucket != "docs" || result.Items[0].Key != "guide.txt" {
		t.Fatalf("search result = %+v", result.Items[0])
	}
}

func TestExtractTextSupportsTextContent(t *testing.T) {
	t.Parallel()

	text, err := index.ExtractText(stringsReader("hello\n\nworld"), model.ObjectMeta{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if text != "hello world" {
		t.Fatalf("text = %q, want %q", text, "hello world")
	}
}

func newSearchEngine(t *testing.T, cfg config.Config) *index.SearchEngine {
	t.Helper()

	engine, err := index.NewSearchEngine(cfg, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("new search engine: %v", err)
	}
	return engine
}

func mustCloseSearchEngine(t *testing.T, engine *index.SearchEngine) {
	t.Helper()
	if err := engine.Close(); err != nil {
		t.Fatalf("close search engine: %v", err)
	}
}

func stringsReader(value string) *strings.Reader {
	return strings.NewReader(value)
}
