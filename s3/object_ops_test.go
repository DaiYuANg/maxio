package s3_test

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	maxios3 "github.com/lyonbrown4d/maxio/s3"
)

type bucketLocationTestResult struct {
	Location string `xml:",chardata"`
}

type copyObjectTestResult struct {
	ETag string `xml:"ETag"`
}

type deleteObjectsTestRequest struct {
	XMLName xml.Name                 `xml:"Delete"`
	Quiet   bool                     `xml:"Quiet,omitempty"`
	Objects []deleteObjectTestTarget `xml:"Object"`
}

type deleteObjectTestTarget struct {
	Key string `xml:"Key"`
}

type deleteObjectsTestResult struct {
	Deleted []deletedObjectTestResult `xml:"Deleted"`
	Errors  []deleteErrorTestResult   `xml:"Error"`
}

type deletedObjectTestResult struct {
	Key string `xml:"Key"`
}

type deleteErrorTestResult struct {
	Key  string `xml:"Key"`
	Code string `xml:"Code"`
}

func TestCopyObjectCopiesBodyAndMetadata(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	sourceHeaders := http.Header{}
	sourceHeaders.Set("Content-Type", "text/plain")
	sourceHeaders.Set("Cache-Control", "max-age=120")
	sourceHeaders.Set("x-amz-meta-owner", "source")
	putObjectWithHeaders(t, service, "/s3/bucket/source.txt", "copied body", sourceHeaders)

	copyHeaders := http.Header{}
	copyHeaders.Set("x-amz-copy-source", "/bucket/source.txt")
	recorder := putObjectWithHeaders(t, service, "/s3/bucket/copy.txt", "", copyHeaders)
	result := copyObjectTestResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode copy result: %v", err)
	}
	if strings.TrimSpace(result.ETag) == "" {
		t.Fatal("copy result etag is empty")
	}

	get := serveS3Request(t, service, http.MethodGet, "/s3/bucket/copy.txt", nil)
	if get.Body.String() != "copied body" {
		t.Fatalf("copied body = %q, want copied body", get.Body.String())
	}
	head := serveS3Request(t, service, http.MethodHead, "/s3/bucket/copy.txt", nil)
	assertHeader(t, head, "Content-Type", "text/plain")
	assertHeader(t, head, "Cache-Control", "max-age=120")
	assertHeader(t, head, "x-amz-meta-owner", "source")
}

func TestCopyObjectCanReplaceMetadata(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	sourceHeaders := http.Header{}
	sourceHeaders.Set("Content-Type", "text/plain")
	sourceHeaders.Set("x-amz-meta-owner", "source")
	putObjectWithHeaders(t, service, "/s3/bucket/source.txt", "replace metadata", sourceHeaders)

	copyHeaders := http.Header{}
	copyHeaders.Set("x-amz-copy-source", "/bucket/source.txt")
	copyHeaders.Set("x-amz-metadata-directive", "REPLACE")
	copyHeaders.Set("Content-Type", "application/json")
	copyHeaders.Set("x-amz-meta-owner", "replacement")
	putObjectWithHeaders(t, service, "/s3/bucket/replaced.txt", "", copyHeaders)

	head := serveS3Request(t, service, http.MethodHead, "/s3/bucket/replaced.txt", nil)
	assertHeader(t, head, "Content-Type", "application/json")
	assertHeader(t, head, "x-amz-meta-owner", "replacement")
}

func TestDeleteObjectsDeletesExistingAndMissingKeys(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	putObjectWithHeaders(t, service, "/s3/bucket/a.txt", "a", nil)
	putObjectWithHeaders(t, service, "/s3/bucket/b.txt", "b", nil)

	result := deleteObjects(t, service, deleteObjectsTestRequest{
		Objects: []deleteObjectTestTarget{
			{Key: "a.txt"},
			{Key: "b.txt"},
			{Key: "missing.txt"},
		},
	})
	if len(result.Errors) != 0 || len(result.Deleted) != 3 {
		t.Fatalf("delete result = %+v", result)
	}
	head := serveS3Request(t, service, http.MethodHead, "/s3/bucket/a.txt", nil)
	if head.Code != http.StatusNotFound {
		t.Fatalf("deleted head status = %d, want %d", head.Code, http.StatusNotFound)
	}
}

func TestDeleteObjectsQuietSuppressesDeletedItems(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	putObjectWithHeaders(t, service, "/s3/bucket/quiet.txt", "quiet", nil)
	result := deleteObjects(t, service, deleteObjectsTestRequest{
		Quiet: true,
		Objects: []deleteObjectTestTarget{
			{Key: "quiet.txt"},
		},
	})
	if len(result.Errors) != 0 || len(result.Deleted) != 0 {
		t.Fatalf("quiet delete result = %+v", result)
	}
}

func TestGetBucketLocationReturnsConfiguredRegion(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	recorder := serveS3Request(t, service, http.MethodGet, "/s3/bucket?location", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("location status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	result := bucketLocationTestResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode location: %v", err)
	}
	if result.Location != "us-east-1" {
		t.Fatalf("location = %q, want us-east-1", result.Location)
	}
}

func putObjectWithHeaders(
	t *testing.T,
	service *maxios3.Service,
	target string,
	body string,
	headers http.Header,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPut, target, strings.NewReader(body))
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("put %s status = %d body = %s", target, recorder.Code, recorder.Body.String())
	}
	return recorder
}

func deleteObjects(
	t *testing.T,
	service *maxios3.Service,
	request deleteObjectsTestRequest,
) deleteObjectsTestResult {
	t.Helper()

	var body strings.Builder
	if err := xml.NewEncoder(&body).Encode(request); err != nil {
		t.Fatalf("encode delete objects: %v", err)
	}
	recorder := serveS3Request(t, service, http.MethodPost, "/s3/bucket?delete", strings.NewReader(body.String()))
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete objects status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	result := deleteObjectsTestResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode delete result: %v", err)
	}
	return result
}
