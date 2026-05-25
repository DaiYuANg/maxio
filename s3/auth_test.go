package s3_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	maxios3 "github.com/lyonbrown4d/maxio/s3"
)

const (
	testSigV4Algorithm = "AWS4-HMAC-SHA256"
	testSigV4Request   = "aws4_request"
	testS3ServiceName  = "s3"
	testSigV4TimeFmt   = "20060102T150405Z"
)

func TestServeHTTPWithSigV4HeaderAllowsValidSignature(t *testing.T) {
	t.Parallel()

	accessID := "maxio-test-client"
	material := strings.Repeat("m", 32)
	region := "us-east-1"
	req := newSignedRequest(t, "http://maxio.local/s3/photos/cat.jpg?versionId=1")
	req.URL.Path = "/s3/%zz"
	signHeaderRequest(t, req, accessID, material, region, "20260516T010203Z")

	recorder := httptest.NewRecorder()
	maxios3.NewService(nil, nil, maxios3.Config{
		AccessKey: accessID,
		SecretKey: material,
		Region:    region,
	}).ServeHTTP(recorder, req)

	if recorder.Code == http.StatusForbidden {
		t.Fatalf("signed header request was rejected: %s", recorder.Body.String())
	}
}

func TestServeHTTPWithPresignedURLAllowsValidSignature(t *testing.T) {
	t.Parallel()

	accessID := "maxio-test-client"
	material := strings.Repeat("m", 32)
	region := "us-east-1"
	req := newSignedRequest(t, "http://maxio.local/s3/photos/cat.jpg?versionId=1")
	req.URL.Path = "/s3/%zz"
	signPresignedRequest(t, req, accessID, material, region, time.Now().UTC(), 60)

	recorder := httptest.NewRecorder()
	maxios3.NewService(nil, nil, maxios3.Config{
		AccessKey: accessID,
		SecretKey: material,
		Region:    region,
	}).ServeHTTP(recorder, req)

	if recorder.Code == http.StatusForbidden {
		t.Fatalf("presigned request was rejected: %s", recorder.Body.String())
	}
}

func TestServeHTTPWithPresignedURLRejectsExpiredSignature(t *testing.T) {
	t.Parallel()

	accessID := "maxio-test-client"
	material := strings.Repeat("m", 32)
	region := "us-east-1"
	req := newSignedRequest(t, "http://maxio.local/s3/photos/cat.jpg")
	req.URL.Path = "/s3/%zz"
	signPresignedRequest(t, req, accessID, material, region, time.Now().UTC().Add(-2*time.Hour), 1)

	recorder := httptest.NewRecorder()
	maxios3.NewService(nil, nil, maxios3.Config{
		AccessKey: accessID,
		SecretKey: material,
		Region:    region,
	}).ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expired presigned request status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func newSignedRequest(t *testing.T, target string) *http.Request {
	t.Helper()

	return httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, http.NoBody)
}

func signHeaderRequest(t *testing.T, req *http.Request, accessID, material, region, amzDate string) {
	t.Helper()

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	date := amzDate[:8]
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	signature := requestSignature(req, material, date, region, amzDate, signedHeaders, req.URL.Query())
	credential := strings.Join([]string{accessID, date, region, testS3ServiceName, testSigV4Request}, "/")
	req.Header.Set("Authorization", testSigV4Algorithm+
		" Credential="+credential+
		", SignedHeaders="+strings.Join(signedHeaders, ";")+
		", Signature="+signature)
}

func signPresignedRequest(
	t *testing.T,
	req *http.Request,
	accessID string,
	material string,
	region string,
	signedAt time.Time,
	expires int,
) {
	t.Helper()

	amzDate := signedAt.Format(testSigV4TimeFmt)
	date := amzDate[:8]
	signedHeaders := []string{"host"}
	query := req.URL.Query()
	query.Set("X-Amz-Algorithm", testSigV4Algorithm)
	query.Set("X-Amz-Credential", strings.Join([]string{accessID, date, region, testS3ServiceName, testSigV4Request}, "/"))
	query.Set("X-Amz-Date", amzDate)
	query.Set("X-Amz-Expires", strconv.Itoa(expires))
	query.Set("X-Amz-SignedHeaders", strings.Join(signedHeaders, ";"))
	req.URL.RawQuery = query.Encode()

	signature := requestSignature(req, material, date, region, amzDate, signedHeaders, queryWithoutSignature(req.URL.Query()))
	query.Set("X-Amz-Signature", signature)
	req.URL.RawQuery = query.Encode()
}

func requestSignature(
	req *http.Request,
	material string,
	date string,
	region string,
	amzDate string,
	signedHeaders []string,
	query url.Values,
) string {
	canonicalRequest := canonicalRequest(req, signedHeaders, query)
	credentialScope := strings.Join([]string{date, region, testS3ServiceName, testSigV4Request}, "/")
	stringToSign := strings.Join([]string{
		testSigV4Algorithm,
		amzDate,
		credentialScope,
		sha256Hex(canonicalRequest),
	}, "\n")
	return signature(material, date, region, testS3ServiceName, stringToSign)
}

func canonicalRequest(req *http.Request, signedHeaders []string, query url.Values) string {
	headers := make([]string, 0, len(signedHeaders))
	for _, header := range signedHeaders {
		headers = append(headers, header+":"+canonicalHeaderValue(req, header))
	}
	return strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		canonicalQuery(query),
		strings.Join(headers, "\n") + "\n",
		strings.Join(signedHeaders, ";"),
		payloadHash(req),
	}, "\n")
}

func canonicalQuery(values url.Values) string {
	pairs := make([]string, 0, len(values))
	for key, items := range values {
		if len(items) == 0 {
			pairs = append(pairs, sigV4Escape(key)+"=")
			continue
		}
		for _, value := range items {
			pairs = append(pairs, sigV4Escape(key)+"="+sigV4Escape(value))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

func queryWithoutSignature(values url.Values) url.Values {
	query := make(url.Values, len(values))
	for key, items := range values {
		if key == "X-Amz-Signature" {
			continue
		}
		query[key] = append([]string(nil), items...)
	}
	return query
}

func canonicalHeaderValue(req *http.Request, header string) string {
	if header == "host" {
		return strings.ToLower(strings.TrimSpace(req.Host))
	}
	return strings.Join(strings.Fields(strings.Join(req.Header.Values(header), ",")), " ")
}

func payloadHash(req *http.Request) string {
	hash := strings.TrimSpace(req.Header.Get("X-Amz-Content-Sha256"))
	if hash == "" {
		return "UNSIGNED-PAYLOAD"
	}
	return hash
}

func sigV4Escape(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "%7E", "~")
	return escaped
}

func signature(material, date, region, service, stringToSign string) string {
	dateValue := hmacSHA256([]byte("AWS4"+material), date)
	regionValue := hmacSHA256(dateValue, region)
	serviceValue := hmacSHA256(regionValue, service)
	signingValue := hmacSHA256(serviceValue, testSigV4Request)
	return hex.EncodeToString(hmacSHA256(signingValue, stringToSign))
}

func hmacSHA256(input []byte, data string) []byte {
	mac := hmac.New(sha256.New, input)
	if _, err := mac.Write([]byte(data)); err != nil {
		return nil
	}
	return mac.Sum(nil)
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
