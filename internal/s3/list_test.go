package s3_test

import (
	"context"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	maxios3 "github.com/lyonbrown4d/maxio/internal/s3"
	"github.com/lyonbrown4d/maxio/object"
)

type listObjectsV2TestResult struct {
	KeyCount              int                      `xml:"KeyCount"`
	MaxKeys               int                      `xml:"MaxKeys"`
	IsTruncated           bool                     `xml:"IsTruncated"`
	NextContinuationToken string                   `xml:"NextContinuationToken"`
	Contents              []listObjectTestResult   `xml:"Contents"`
	CommonPrefixes        []commonPrefixTestResult `xml:"CommonPrefixes"`
}

type listObjectTestResult struct {
	Key  string `xml:"Key"`
	ETag string `xml:"ETag"`
}

type commonPrefixTestResult struct {
	Prefix string `xml:"Prefix"`
}

func TestListObjectsV2PaginatesWithContinuationToken(t *testing.T) {
	t.Parallel()

	service, objects := newMultipartTestService(t)
	putListObject(t, objects, "a.txt")
	putListObject(t, objects, "b.txt")
	putListObject(t, objects, "c.txt")

	first := listObjectsV2(t, service, "/s3/bucket?list-type=2&max-keys=2")
	if first.KeyCount != 2 || !first.IsTruncated || len(first.Contents) != 2 {
		t.Fatalf("first page = %+v", first)
	}
	if first.NextContinuationToken == "" {
		t.Fatal("next continuation token is empty")
	}

	second := listObjectsV2(t, service, "/s3/bucket?list-type=2&max-keys=2&continuation-token="+first.NextContinuationToken)
	if second.KeyCount != 1 || second.IsTruncated || len(second.Contents) != 1 {
		t.Fatalf("second page = %+v", second)
	}
	if second.Contents[0].Key != "c.txt" {
		t.Fatalf("second page key = %q, want c.txt", second.Contents[0].Key)
	}
}

func TestListObjectsV2SupportsDelimiter(t *testing.T) {
	t.Parallel()

	service, objects := newMultipartTestService(t)
	putListObject(t, objects, "photos/2026/a.jpg")
	putListObject(t, objects, "photos/2026/b.jpg")
	putListObject(t, objects, "photos/readme.txt")

	result := listObjectsV2(t, service, "/s3/bucket?list-type=2&prefix=photos/&delimiter=/")
	if result.KeyCount != 2 {
		t.Fatalf("key count = %d, want 2", result.KeyCount)
	}
	if len(result.CommonPrefixes) != 1 || result.CommonPrefixes[0].Prefix != "photos/2026/" {
		t.Fatalf("common prefixes = %+v", result.CommonPrefixes)
	}
	if len(result.Contents) != 1 || result.Contents[0].Key != "photos/readme.txt" {
		t.Fatalf("contents = %+v", result.Contents)
	}
}

func putListObject(t *testing.T, objects *object.Service, key string) {
	t.Helper()

	_, err := objects.PutObject(context.Background(), "bucket", key, strings.NewReader("content"), object.PutOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("put list object %q: %v", key, err)
	}
}

func listObjectsV2(t *testing.T, service *maxios3.Service, target string) listObjectsV2TestResult {
	t.Helper()

	recorder := serveS3Request(t, service, http.MethodGet, target, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	result := listObjectsV2TestResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode list result: %v", err)
	}
	return result
}
