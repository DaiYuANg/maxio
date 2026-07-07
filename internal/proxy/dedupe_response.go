package proxy

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"

	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/internal/model"
	"github.com/samber/oops"
)

type proxyResponseBuffer struct {
	header http.Header
	status int
	body   bytes.Buffer
}

type s3ErrorResponse struct {
	XMLName xml.Name `xml:"Error"`
	Code    string   `xml:"Code"`
	Message string   `xml:"Message"`
}

func newProxyResponseBuffer() *proxyResponseBuffer {
	return &proxyResponseBuffer{header: make(http.Header), status: http.StatusOK}
}

func (r *proxyResponseBuffer) Header() http.Header {
	return r.header
}

func (r *proxyResponseBuffer) WriteHeader(status int) {
	r.status = status
}

func (r *proxyResponseBuffer) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.body.Write(data)
	if err != nil {
		return n, oops.Wrapf(err, "buffer proxy response")
	}
	return n, nil
}

func (r *proxyResponseBuffer) Flush() {}

func (r *proxyResponseBuffer) statusCode() int {
	if r == nil || r.status == 0 {
		return http.StatusOK
	}
	return r.status
}

func (r *proxyResponseBuffer) Send(w http.ResponseWriter) error {
	for key, values := range r.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(r.statusCode())
	if r.body.Len() == 0 {
		return nil
	}
	if _, err := w.Write(r.body.Bytes()); err != nil {
		return oops.Wrapf(err, "write buffered proxy response")
	}
	return nil
}

func parseS3ObjectPath(rawPath string) (string, string, bool) {
	objectPath := strings.TrimLeft(rawPath, "/")
	bucket, key, ok := strings.Cut(objectPath, "/")
	if !ok {
		return "", "", false
	}
	bucket = strings.TrimSpace(bucket)
	return bucket, key, bucket != "" && key != ""
}

func s3ObjectPath(bucket, key string) string {
	return "/" + strings.Trim(strings.TrimSpace(bucket), "/") + "/" + key
}

func isSuccessfulProxyStatus(status int) bool {
	return status >= http.StatusOK && status < http.StatusMultipleChoices
}

func objectVersionIDFromResponse(header http.Header, digest string) string {
	if header != nil {
		if versionID := strings.TrimSpace(header.Get("x-amz-version-id")); versionID != "" {
			return versionID
		}
	}
	shortDigest := strings.TrimPrefix(digest, "sha256:")
	if len(shortDigest) > 16 {
		shortDigest = shortDigest[:16]
	}
	return fmt.Sprintf("proxy-%d-%s", time.Now().UTC().UnixNano(), shortDigest)
}

func digestETag(digest string) string {
	value := strings.TrimSpace(strings.TrimPrefix(digest, "sha256:"))
	if value == "" {
		value = strings.TrimSpace(digest)
	}
	return `"` + value + `"`
}

func userMetadataFromHeader(header http.Header) map[string]string {
	metadataValues := make(map[string]string)
	for key, values := range header {
		if !strings.HasPrefix(strings.ToLower(key), "x-amz-meta-") || len(values) == 0 {
			continue
		}
		metadataKey := strings.TrimPrefix(strings.ToLower(key), "x-amz-meta-")
		metadataValue := strings.TrimSpace(values[0])
		if metadataKey != "" && metadataValue != "" {
			metadataValues[metadataKey] = metadataValue
		}
	}
	if len(metadataValues) == 0 {
		return nil
	}
	return metadataValues
}

func newObjectPutEventFromRequest(
	r *http.Request,
	upstreamID string,
	bucket string,
	key string,
	captured *capturedRequestBody,
) ObjectPutSucceededEvent {
	return ObjectPutSucceededEvent{
		Bucket:         bucket,
		Key:            key,
		Digest:         captured.digest,
		Size:           captured.size,
		ETag:           digestETag(captured.digest),
		ContentType:    r.Header.Get("Content-Type"),
		UpstreamID:     upstreamID,
		UpstreamBucket: bucket,
		UpstreamKey:    key,
		UserMetadata:   userMetadataFromHeader(r.Header),
		DedupeHit:      false,
	}
}

func newObjectPutEventFromDigestHit(
	r *http.Request,
	bucket string,
	key string,
	captured *capturedRequestBody,
	ref model.DigestRef,
) ObjectPutSucceededEvent {
	return ObjectPutSucceededEvent{
		Bucket:         bucket,
		Key:            key,
		Digest:         captured.digest,
		Size:           captured.size,
		ETag:           digestETag(captured.digest),
		ContentType:    r.Header.Get("Content-Type"),
		UpstreamID:     ref.UpstreamID,
		UpstreamBucket: ref.UpstreamBucket,
		UpstreamKey:    ref.UpstreamKey,
		VersionID:      objectVersionIDFromResponse(nil, captured.digest),
		UserMetadata:   userMetadataFromHeader(r.Header),
		DedupeHit:      true,
	}
}

func newObjectPutEventFromUpstreamResponse(
	r *http.Request,
	upstreamID string,
	bucket string,
	key string,
	captured *capturedRequestBody,
	response *proxyResponseBuffer,
) ObjectPutSucceededEvent {
	return ObjectPutSucceededEvent{
		Bucket:         bucket,
		Key:            key,
		Digest:         captured.digest,
		Size:           captured.size,
		ETag:           response.Header().Get("ETag"),
		ContentType:    r.Header.Get("Content-Type"),
		UpstreamID:     upstreamID,
		UpstreamBucket: bucket,
		UpstreamKey:    key,
		VersionID:      objectVersionIDFromResponse(response.Header(), captured.digest),
		UserMetadata:   userMetadataFromHeader(r.Header),
		DedupeHit:      false,
	}
}

func writeS3ProxyInternalError(w http.ResponseWriter, message string) {
	writeS3ProxyError(w, http.StatusInternalServerError, "InternalError", message)
}

func writeS3ProxyError(w http.ResponseWriter, status int, code, message string) {
	payload, err := xml.Marshal(s3ErrorResponse{Code: code, Message: message})
	if err != nil {
		payload = []byte("<Error><Code>InternalError</Code><Message>internal error</Message></Error>")
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	if _, err := w.Write(payload); err != nil {
		return
	}
}
