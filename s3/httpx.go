package s3

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"

	"github.com/arcgolabs/httpx"
	"github.com/lyonbrown4d/maxio/object"
)

type httpxServiceInput struct{}

type httpxBucketInput struct {
	Bucket            string `path:"bucket"`
	Prefix            string `query:"prefix"`
	MaxKeys           int    `query:"max-keys"`
	ListType          string `query:"list-type"`
	ContinuationToken string `query:"continuation-token"`
	StartAfter        string `query:"start-after"`
	Delimiter         string `query:"delimiter"`
}

type httpxObjectInput struct {
	Bucket             string `path:"bucket"`
	Key                string `path:"key"`
	ContentType        string `header:"Content-Type"`
	CacheControl       string `header:"Cache-Control"`
	ContentDisposition string `header:"Content-Disposition"`
	ContentEncoding    string `header:"Content-Encoding"`
	ContentLanguage    string `header:"Content-Language"`
	Range              string `header:"Range"`
	Payload            httpx.RequestStream
}

type httpxOutput struct {
	Status             int    `status:"200"`
	RequestID          string `header:"x-amz-request-id"`
	ContentType        string `header:"Content-Type"`
	ContentLength      string `header:"Content-Length"`
	ContentRange       string `header:"Content-Range"`
	AcceptRanges       string `header:"Accept-Ranges"`
	LastModified       string `header:"Last-Modified"`
	ETag               string `header:"ETag"`
	Location           string `header:"Location"`
	CacheControl       string `header:"Cache-Control"`
	ContentDisposition string `header:"Content-Disposition"`
	ContentEncoding    string `header:"Content-Encoding"`
	ContentLanguage    string `header:"Content-Language"`
	Body               httpx.ResponseStream
}

type Endpoint struct {
	service *Service
}

func NewEndpoint(service *Service) *Endpoint {
	return &Endpoint{service: service}
}

func (e *Endpoint) EndpointSpec() httpx.EndpointSpec {
	return httpx.EndpointSpec{
		Prefix:        compatPrefix,
		Tags:          httpx.Tags("s3"),
		SummaryPrefix: "S3",
	}
}

func (e *Endpoint) Register(registrar httpx.Registrar) {
	group := registrar.Scope()

	httpx.MustGroupGet[httpxServiceInput, httpxOutput](
		group,
		"",
		e.service.listBucketsHTTPX,
		httpx.OperationBinaryResponse(contentTypeXML),
	)
	httpx.MustGroupRoute[httpxBucketInput, httpxOutput](group, httpx.MethodHead, "/{bucket}", e.service.headBucketHTTPX)
	httpx.MustGroupGet[httpxBucketInput, httpxOutput](
		group,
		"/{bucket}",
		e.service.listObjectsHTTPX,
		httpx.OperationBinaryResponse(contentTypeXML),
	)
	httpx.MustGroupPut[httpxBucketInput, httpxOutput](group, "/{bucket}", e.service.createBucketHTTPX)
	httpx.MustGroupDelete[httpxBucketInput, httpxOutput](group, "/{bucket}", e.service.deleteBucketHTTPX)
	httpx.MustGroupRoute[httpxObjectInput, httpxOutput](group, httpx.MethodHead, "/{bucket}/{key...}", e.service.headObjectHTTPX)
	httpx.MustGroupGet[httpxObjectInput, httpxOutput](
		group,
		"/{bucket}/{key...}",
		e.service.getObjectHTTPX,
		httpx.OperationBinaryResponse("application/octet-stream"),
	)
	httpx.MustGroupPut[httpxObjectInput, httpxOutput](
		group,
		"/{bucket}/{key...}",
		e.service.putObjectHTTPX,
		httpx.OperationBinaryRequest("application/octet-stream"),
		httpx.OperationBinaryResponse("application/octet-stream"),
	)
	httpx.MustGroupDelete[httpxObjectInput, httpxOutput](group, "/{bucket}/{key...}", e.service.deleteObjectHTTPX)
}

func (s *Service) listBucketsHTTPX(ctx context.Context, _ *httpxServiceInput) (*httpxOutput, error) {
	buckets, err := s.objects.ListBuckets(ctx)
	if err != nil {
		return s.mappedErrorHTTPX(err)
	}

	result := listAllMyBucketsResult{
		XMLNS: defaultXMLNS,
		Owner: owner{
			ID:          "maxio",
			DisplayName: "maxio",
		},
		Buckets: make([]bucketResult, 0, len(buckets)),
	}
	for _, bucket := range buckets {
		result.Buckets = append(result.Buckets, bucketResult{
			Name:         bucket.Name,
			CreationDate: formatS3Time(bucket.CreatedAt),
		})
	}
	return s.xmlHTTPX(http.StatusOK, result)
}

func (s *Service) headBucketHTTPX(ctx context.Context, input *httpxBucketInput) (*httpxOutput, error) {
	if _, err := s.objects.ListObjects(ctx, input.Bucket, ""); err != nil {
		return s.mappedErrorHTTPX(err)
	}
	return s.emptyHTTPX(http.StatusOK), nil
}

func (s *Service) createBucketHTTPX(ctx context.Context, input *httpxBucketInput) (*httpxOutput, error) {
	if err := s.objects.CreateBucket(ctx, input.Bucket); err != nil {
		return s.mappedErrorHTTPX(err)
	}
	out := s.emptyHTTPX(http.StatusOK)
	out.Location = "/" + input.Bucket
	return out, nil
}

func (s *Service) deleteBucketHTTPX(ctx context.Context, input *httpxBucketInput) (*httpxOutput, error) {
	if err := s.objects.DeleteBucket(ctx, input.Bucket); err != nil {
		return s.mappedErrorHTTPX(err)
	}
	return s.emptyHTTPX(http.StatusNoContent), nil
}

func (s *Service) listObjectsHTTPX(ctx context.Context, input *httpxBucketInput) (*httpxOutput, error) {
	result, err := s.listObjectsResult(ctx, input.Bucket, listObjectsOptionsFromHTTPX(input))
	if err != nil {
		return s.mappedErrorHTTPX(err)
	}
	return s.xmlHTTPX(http.StatusOK, result)
}

func (s *Service) headObjectHTTPX(ctx context.Context, input *httpxObjectInput) (*httpxOutput, error) {
	meta, err := s.objects.StatObject(ctx, input.Bucket, input.Key)
	if err != nil {
		return s.mappedErrorHTTPX(err)
	}
	return s.objectHeadersHTTPX(http.StatusOK, meta), nil
}

func (s *Service) getObjectHTTPX(ctx context.Context, input *httpxObjectInput) (*httpxOutput, error) {
	body, meta, err := s.objects.GetObject(ctx, input.Bucket, input.Key)
	if err != nil {
		return s.mappedErrorHTTPX(err)
	}

	return s.rangedObjectHTTPX(ctx, input.Range, body, meta)
}

func (s *Service) putObjectHTTPX(ctx context.Context, input *httpxObjectInput) (*httpxOutput, error) {
	meta, err := s.objects.PutObject(ctx, input.Bucket, input.Key, input.Payload.Reader(), object.PutOptions{
		ContentType:        input.ContentType,
		CacheControl:       input.CacheControl,
		ContentDisposition: input.ContentDisposition,
		ContentEncoding:    input.ContentEncoding,
		ContentLanguage:    input.ContentLanguage,
	})
	if err != nil {
		return s.mappedErrorHTTPX(err)
	}
	return s.objectHeadersHTTPX(http.StatusOK, meta), nil
}

func (s *Service) deleteObjectHTTPX(ctx context.Context, input *httpxObjectInput) (*httpxOutput, error) {
	if _, err := s.objects.DeleteObject(ctx, input.Bucket, input.Key); err != nil {
		return s.mappedErrorHTTPX(err)
	}
	return s.emptyHTTPX(http.StatusNoContent), nil
}

func (s *Service) mappedErrorHTTPX(err error) (*httpxOutput, error) {
	status, code := mapError(err)
	return s.errorHTTPX(status, code, err.Error())
}

func (s *Service) errorHTTPX(status int, code, message string) (*httpxOutput, error) {
	requestID := requestID()
	return s.xmlHTTPXWithRequestID(status, requestID, errorResult{
		Code:      code,
		Message:   message,
		RequestID: requestID,
	})
}

func (s *Service) xmlHTTPX(status int, value any) (*httpxOutput, error) {
	return s.xmlHTTPXWithRequestID(status, requestID(), value)
}

func (s *Service) xmlHTTPXWithRequestID(status int, requestID string, value any) (*httpxOutput, error) {
	body, err := encodeXMLHTTPX(value)
	if err != nil {
		return nil, fmt.Errorf("encode s3 xml response: %w", err)
	}
	out := s.emptyHTTPX(status)
	out.RequestID = requestID
	out.ContentType = contentTypeXML
	out.Body = httpx.StreamBytes(body)
	return out, nil
}

func (s *Service) emptyHTTPX(status int) *httpxOutput {
	return &httpxOutput{
		Status:    status,
		RequestID: requestID(),
	}
}

func (s *Service) objectHeadersHTTPX(status int, meta object.ObjectMeta) *httpxOutput {
	out := s.emptyHTTPX(status)
	out.ETag = meta.ETag
	out.ContentLength = strconv.FormatInt(meta.Size, 10)
	out.AcceptRanges = "bytes"
	out.LastModified = meta.UpdatedAt.UTC().Format(http.TimeFormat)
	if meta.ContentType != "" {
		out.ContentType = meta.ContentType
	}
	out.CacheControl = meta.CacheControl
	out.ContentDisposition = meta.ContentDisposition
	out.ContentEncoding = meta.ContentEncoding
	out.ContentLanguage = meta.ContentLanguage
	return out
}

func encodeXMLHTTPX(value any) ([]byte, error) {
	var buf bytes.Buffer
	if _, err := buf.WriteString(xml.Header); err != nil {
		return nil, fmt.Errorf("write xml header: %w", err)
	}
	if err := xml.NewEncoder(&buf).Encode(value); err != nil {
		return nil, fmt.Errorf("encode xml body: %w", err)
	}
	return buf.Bytes(), nil
}

func normalizeHTTPXMaxKeys(value int) int {
	if value <= 0 {
		return 1000
	}
	return value
}
