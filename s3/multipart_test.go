package s3_test

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/index"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/object"
	maxios3 "github.com/lyonbrown4d/maxio/s3"
)

type initiateMultipartResult struct {
	UploadID string `xml:"UploadId"`
}

type completeMultipartResult struct {
	ETag string `xml:"ETag"`
}

type completeMultipartRequest struct {
	XMLName xml.Name                `xml:"CompleteMultipartUpload"`
	Parts   []completeMultipartPart `xml:"Part"`
}

type completeMultipartPart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

func TestMultipartUploadCompletesObject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service, objects := newMultipartTestService(t)
	uploadID := initiateMultipartUpload(t, service, "/s3/bucket/object.txt?uploads")
	firstPart := strings.Repeat("a", 5*1024*1024)
	partOneETag := uploadPart(t, service, "/s3/bucket/object.txt?partNumber=1&uploadId="+uploadID, firstPart)
	partTwoETag := uploadPart(t, service, "/s3/bucket/object.txt?partNumber=2&uploadId="+uploadID, "world")
	result := completeMultipartUpload(t, service, "/s3/bucket/object.txt?uploadId="+uploadID, completeMultipartRequest{
		Parts: []completeMultipartPart{
			{PartNumber: 1, ETag: partOneETag},
			{PartNumber: 2, ETag: partTwoETag},
		},
	})
	if !strings.Contains(result.ETag, "-2") {
		t.Fatalf("complete etag = %q, want multipart suffix", result.ETag)
	}

	body, _, err := objects.GetObject(ctx, "bucket", "object.txt")
	if err != nil {
		t.Fatalf("get completed object: %v", err)
	}
	defer closeReadCloser(t, body)
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read completed object: %v", err)
	}
	if len(data) != len(firstPart)+len("world") || !strings.HasSuffix(string(data), "world") {
		t.Fatalf("object body length = %d, want suffix world and length %d", len(data), len(firstPart)+len("world"))
	}
}

func TestMultipartUploadAbortRemovesUpload(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	uploadID := initiateMultipartUpload(t, service, "/s3/bucket/aborted.txt?uploads")
	uploadPart(t, service, "/s3/bucket/aborted.txt?partNumber=1&uploadId="+uploadID, "staged")

	recorder := serveS3Request(t, service, http.MethodDelete, "/s3/bucket/aborted.txt?uploadId="+uploadID, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("abort status = %d, want %d", recorder.Code, http.StatusNoContent)
	}

	recorder = serveS3Request(t, service, http.MethodDelete, "/s3/bucket/aborted.txt?uploadId="+uploadID, nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("second abort status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func newMultipartTestService(t *testing.T) (*maxios3.Service, *object.Service) {
	t.Helper()

	ctx := context.Background()
	storage, err := store.NewStore(t.TempDir(), metadata.NewInMemoryMetadata(), nil)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	objects := object.NewService(storage, index.NewInMemorySearchEngine(), nil, slog.New(slog.DiscardHandler), config.Config{})
	if err := objects.CreateBucket(ctx, "bucket"); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	return maxios3.NewService(objects, slog.New(slog.DiscardHandler), maxios3.Config{DataDir: t.TempDir()}), objects
}

func initiateMultipartUpload(t *testing.T, service *maxios3.Service, target string) string {
	t.Helper()

	recorder := serveS3Request(t, service, http.MethodPost, target, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("initiate status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	result := initiateMultipartResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode initiate result: %v", err)
	}
	if strings.TrimSpace(result.UploadID) == "" {
		t.Fatal("upload id is empty")
	}
	return result.UploadID
}

func uploadPart(t *testing.T, service *maxios3.Service, target, body string) string {
	t.Helper()

	recorder := serveS3Request(t, service, http.MethodPut, target, strings.NewReader(body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload part status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	etag := recorder.Header().Get("ETag")
	if etag == "" {
		t.Fatal("part etag is empty")
	}
	return etag
}

func completeMultipartUpload(
	t *testing.T,
	service *maxios3.Service,
	target string,
	request completeMultipartRequest,
) completeMultipartResult {
	t.Helper()

	var body bytes.Buffer
	if err := xml.NewEncoder(&body).Encode(request); err != nil {
		t.Fatalf("encode complete request: %v", err)
	}
	recorder := serveS3Request(t, service, http.MethodPost, target, &body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("complete status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	result := completeMultipartResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode complete result: %v", err)
	}
	return result
}

func serveS3Request(
	t *testing.T,
	service *maxios3.Service,
	method string,
	target string,
	body io.Reader,
) *httptest.ResponseRecorder {
	t.Helper()

	if body == nil {
		body = http.NoBody
	}
	request := httptest.NewRequestWithContext(context.Background(), method, target, body)
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	return recorder
}

func closeReadCloser(t *testing.T, body io.ReadCloser) {
	t.Helper()
	if err := body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
}
