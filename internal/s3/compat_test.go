package s3_test

import (
	"context"
	"encoding/xml"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lyonbrown4d/maxio/internal/config"
	maxios3 "github.com/lyonbrown4d/maxio/internal/s3"
)

type s3ErrorTestResult struct {
	Code      string `xml:"Code"`
	Message   string `xml:"Message"`
	RequestID string `xml:"RequestId"`
}

type listPartsTestResult struct {
	NextPartNumberMarker int                  `xml:"NextPartNumberMarker"`
	MaxParts             int                  `xml:"MaxParts"`
	IsTruncated          bool                 `xml:"IsTruncated"`
	Parts                []listPartTestResult `xml:"Part"`
}

type listPartTestResult struct {
	PartNumber int `xml:"PartNumber"`
}

type listMultipartUploadsTestResult struct {
	IsTruncated bool                            `xml:"IsTruncated"`
	Uploads     []listMultipartUploadTestResult `xml:"Upload"`
}

type listMultipartUploadTestResult struct {
	Key      string `xml:"Key"`
	UploadID string `xml:"UploadId"`
}

func TestS3ObjectMetadataHeadersRoundTrip(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/s3/bucket/meta.txt", strings.NewReader("metadata"))
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Cache-Control", "max-age=60")
	request.Header.Set("Content-Disposition", "attachment")
	request.Header.Set("Content-Encoding", "identity")
	request.Header.Set("Content-Language", "en")
	request.Header.Set("x-amz-meta-owner", "maxio")
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("put status = %d body = %s", recorder.Code, recorder.Body.String())
	}

	head := serveS3Request(t, service, http.MethodHead, "/s3/bucket/meta.txt", nil)
	assertHeader(t, head, "Content-Type", "text/plain")
	assertHeader(t, head, "Cache-Control", "max-age=60")
	assertHeader(t, head, "Content-Disposition", "attachment")
	assertHeader(t, head, "Content-Encoding", "identity")
	assertHeader(t, head, "Content-Language", "en")
	assertHeader(t, head, "x-amz-meta-owner", "maxio")
}

func TestS3PresignedURLReadsObject(t *testing.T) {
	t.Parallel()

	accessID := "maxio-test-client"
	material := strings.Repeat("m", 32)
	region := "us-east-1"
	_, objects := newMultipartTestService(t)
	putListObject(t, objects, "presigned.txt")
	service := maxios3.NewService(objects, slog.New(slog.DiscardHandler), config.Config{
		DataDir:     t.TempDir(),
		S3AccessKey: accessID,
		S3SecretKey: material,
		S3Region:    region,
	})
	request := newSignedRequest(t, "http://maxio.local/s3/bucket/presigned.txt?response-content-type=text%2Fplain")
	signPresignedRequest(t, request, accessID, material, region, time.Now().UTC(), 60)

	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("presigned get status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "content" {
		t.Fatalf("presigned get body = %q, want content", recorder.Body.String())
	}
}

func TestS3RangeGetReturnsPartialContent(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	putObjectWithHeaders(t, service, "/s3/bucket/range.txt", "0123456789", nil)

	recorder := getObjectWithRange(t, service, "/s3/bucket/range.txt", "bytes=2-6")
	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "23456" {
		t.Fatalf("range body = %q, want 23456", recorder.Body.String())
	}
	assertHeader(t, recorder, "Accept-Ranges", "bytes")
	assertHeader(t, recorder, "Content-Length", "5")
	assertHeader(t, recorder, "Content-Range", "bytes 2-6/10")
}

func TestS3InvalidRangeReturnsXMLInvalidRange(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	putObjectWithHeaders(t, service, "/s3/bucket/range-error.txt", "0123456789", nil)

	recorder := getObjectWithRange(t, service, "/s3/bucket/range-error.txt", "bytes=20-30")
	if recorder.Code != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("range status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	assertHeader(t, recorder, "Content-Type", "application/xml")
	assertHeader(t, recorder, "Content-Range", "bytes */10")
	result := decodeS3Error(t, recorder)
	if result.Code != "InvalidRange" {
		t.Fatalf("error code = %q, want InvalidRange", result.Code)
	}
	if strings.TrimSpace(result.Message) == "" || strings.TrimSpace(result.RequestID) == "" {
		t.Fatalf("error result missing message or request id: %+v", result)
	}
}

func TestS3ETagIsQuotedAndStableAcrossObjectResponses(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	put := putObjectWithHeaders(t, service, "/s3/bucket/etag.txt", "etag-compatible-body", nil)
	etag := put.Header().Get("ETag")
	if !isQuotedETag(etag) {
		t.Fatalf("put etag = %q, want quoted etag", etag)
	}

	head := serveS3Request(t, service, http.MethodHead, "/s3/bucket/etag.txt", nil)
	assertHeader(t, head, "ETag", etag)

	get := serveS3Request(t, service, http.MethodGet, "/s3/bucket/etag.txt", nil)
	assertHeader(t, get, "ETag", etag)

	list := listObjectsV2(t, service, "/s3/bucket?list-type=2&prefix=etag.txt")
	if len(list.Contents) != 1 {
		t.Fatalf("list contents = %+v, want one object", list.Contents)
	}
	if list.Contents[0].ETag != etag {
		t.Fatalf("list etag = %q, want %q", list.Contents[0].ETag, etag)
	}
}

func TestMultipartCompleteRejectsSmallNonLastPart(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	uploadID := initiateMultipartUpload(t, service, "/s3/bucket/small.txt?uploads")
	firstETag := uploadPart(t, service, "/s3/bucket/small.txt?partNumber=1&uploadId="+uploadID, "small")
	secondETag := uploadPart(t, service, "/s3/bucket/small.txt?partNumber=2&uploadId="+uploadID, "last")
	recorder := completeMultipartUploadRaw(t, service, "/s3/bucket/small.txt?uploadId="+uploadID, completeMultipartRequest{
		Parts: []completeMultipartPart{
			{PartNumber: 1, ETag: firstETag},
			{PartNumber: 2, ETag: secondETag},
		},
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("complete status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	result := decodeS3Error(t, recorder)
	if result.Code != "EntityTooSmall" {
		t.Fatalf("error code = %q, want EntityTooSmall", result.Code)
	}
}

func TestListPartsPaginates(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	uploadID := initiateMultipartUpload(t, service, "/s3/bucket/parts.txt?uploads")
	uploadPart(t, service, "/s3/bucket/parts.txt?partNumber=1&uploadId="+uploadID, "one")
	uploadPart(t, service, "/s3/bucket/parts.txt?partNumber=2&uploadId="+uploadID, "two")

	first := listParts(t, service, "/s3/bucket/parts.txt?uploadId="+uploadID+"&max-parts=1")
	if !first.IsTruncated || first.NextPartNumberMarker != 1 || len(first.Parts) != 1 {
		t.Fatalf("first page = %+v", first)
	}
	second := listParts(t, service, "/s3/bucket/parts.txt?uploadId="+uploadID+"&part-number-marker=1&max-parts=1")
	if second.IsTruncated || len(second.Parts) != 1 || second.Parts[0].PartNumber != 2 {
		t.Fatalf("second page = %+v", second)
	}
}

func TestListMultipartUploadsFiltersByPrefix(t *testing.T) {
	t.Parallel()

	service, _ := newMultipartTestService(t)
	photosUploadID := initiateMultipartUpload(t, service, "/s3/bucket/photos/a.txt?uploads")
	initiateMultipartUpload(t, service, "/s3/bucket/docs/a.txt?uploads")

	result := listMultipartUploads(t, service, "/s3/bucket?uploads&prefix=photos/")
	if result.IsTruncated || len(result.Uploads) != 1 {
		t.Fatalf("uploads = %+v", result)
	}
	if result.Uploads[0].Key != "photos/a.txt" || result.Uploads[0].UploadID != photosUploadID {
		t.Fatalf("upload = %+v, want photos upload %q", result.Uploads[0], photosUploadID)
	}
}

func completeMultipartUploadRaw(
	t *testing.T,
	service *maxios3.Service,
	target string,
	request completeMultipartRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	var body strings.Builder
	if err := xml.NewEncoder(&body).Encode(request); err != nil {
		t.Fatalf("encode complete request: %v", err)
	}
	return serveS3Request(t, service, http.MethodPost, target, strings.NewReader(body.String()))
}

func decodeS3Error(t *testing.T, recorder *httptest.ResponseRecorder) s3ErrorTestResult {
	t.Helper()

	result := s3ErrorTestResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode error result: %v", err)
	}
	return result
}

func listParts(t *testing.T, service *maxios3.Service, target string) listPartsTestResult {
	t.Helper()

	recorder := serveS3Request(t, service, http.MethodGet, target, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list parts status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	result := listPartsTestResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode list parts result: %v", err)
	}
	return result
}

func listMultipartUploads(
	t *testing.T,
	service *maxios3.Service,
	target string,
) listMultipartUploadsTestResult {
	t.Helper()

	recorder := serveS3Request(t, service, http.MethodGet, target, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("list uploads status = %d body = %s", recorder.Code, recorder.Body.String())
	}
	result := listMultipartUploadsTestResult{}
	if err := xml.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("decode list uploads result: %v", err)
	}
	return result
}

func getObjectWithRange(
	t *testing.T,
	service *maxios3.Service,
	target string,
	rangeHeader string,
) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
	request.Header.Set("Range", rangeHeader)
	recorder := httptest.NewRecorder()
	service.ServeHTTP(recorder, request)
	return recorder
}

func isQuotedETag(value string) bool {
	return len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`)
}

func assertHeader(t *testing.T, recorder *httptest.ResponseRecorder, name, want string) {
	t.Helper()
	if got := recorder.Header().Get(name); got != want {
		t.Fatalf("header %s = %q, want %q", name, got, want)
	}
}
