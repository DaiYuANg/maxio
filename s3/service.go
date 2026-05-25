package s3

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lyonbrown4d/maxio/object"
)

const (
	amzRequestIDHeader = "x-amz-request-id"
	contentTypeXML     = "application/xml"
	defaultPathPrefix  = "/s3"
)

// ObjectStore is the object-service surface required by the S3 compatibility layer.
type ObjectStore interface {
	ListBuckets(ctx context.Context) ([]object.Bucket, error)
	CreateBucket(ctx context.Context, name string) error
	DeleteBucket(ctx context.Context, name string) error
	ListObjects(ctx context.Context, bucket, prefix string) ([]object.ObjectMeta, error)
	PutObject(ctx context.Context, bucket, key string, reader io.Reader, opts object.PutOptions) (object.ObjectMeta, error)
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, object.ObjectMeta, error)
	StatObject(ctx context.Context, bucket, key string) (object.ObjectMeta, error)
	DeleteObject(ctx context.Context, bucket, key string) (object.ObjectMeta, error)
}

type Service struct {
	objects   ObjectStore
	logger    *slog.Logger
	cfg       Config
	multipart *multipartStore
}

func (s *Service) Match(r *http.Request) bool {
	if r == nil {
		return false
	}
	if isCompatPrefix(r.URL.Path, s.PathPrefix()) {
		return true
	}
	if isReservedNativePath(r.URL.Path) {
		return false
	}
	if hasS3Query(r.URL.Query()) || hasS3Header(r.Header) {
		return true
	}
	auth := r.Header.Get("Authorization")
	return strings.HasPrefix(auth, "AWS ") || strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ")
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(amzRequestIDHeader, requestID())
	if err := s.authorize(r); err != nil {
		s.writeError(w, http.StatusForbidden, "AccessDenied", err.Error())
		return
	}

	bucket, key, err := splitS3Path(r.URL.Path, s.PathPrefix())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "InvalidURI", err.Error())
		return
	}
	switch {
	case bucket == "":
		s.handleService(w, r)
	case key == "":
		s.handleBucket(w, r, bucket)
	default:
		s.handleObject(w, r, bucket, key)
	}
}

func (s *Service) handleService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		return
	}
	buckets, err := s.objects.ListBuckets(r.Context())
	if err != nil {
		s.writeMappedError(w, err)
		return
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
	s.writeXML(w, http.StatusOK, result)
}

func (s *Service) handleBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	switch r.Method {
	case http.MethodHead:
		s.handleHeadBucket(w, r, bucket)
	case http.MethodGet:
		if hasQueryKey(r.URL.Query(), "location") {
			s.handleGetBucketLocation(w, r, bucket)
			return
		}
		if hasQueryKey(r.URL.Query(), "uploads") {
			s.handleListMultipartUploads(w, r, bucket)
			return
		}
		s.handleListObjects(w, r, bucket)
	case http.MethodPost:
		if hasQueryKey(r.URL.Query(), "delete") {
			s.handleDeleteObjects(w, r, bucket)
			return
		}
		s.writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	case http.MethodPut:
		s.handleCreateBucket(w, r, bucket)
	case http.MethodDelete:
		s.handleDeleteBucket(w, r, bucket)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	}
}

func (s *Service) handleHeadBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if _, err := s.objects.ListObjects(r.Context(), bucket, ""); err != nil {
		s.writeMappedError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Service) handleCreateBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := s.objects.CreateBucket(r.Context(), bucket); err != nil {
		s.writeMappedError(w, err)
		return
	}
	w.Header().Set("Location", "/"+bucket)
	w.WriteHeader(http.StatusOK)
}

func (s *Service) handleDeleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	if err := s.objects.DeleteBucket(r.Context(), bucket); err != nil {
		s.writeMappedError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleListObjects(w http.ResponseWriter, r *http.Request, bucket string) {
	result, err := s.listObjectsResult(r.Context(), bucket, listObjectsOptionsFromQuery(r.URL.Query()))
	if err != nil {
		s.writeMappedError(w, err)
		return
	}
	s.writeXML(w, http.StatusOK, result)
}

func (s *Service) handleObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if s.handleMultipartObject(w, r, bucket, key) {
		return
	}

	switch r.Method {
	case http.MethodHead:
		s.handleHeadObject(w, r, bucket, key)
	case http.MethodGet:
		s.handleGetObject(w, r, bucket, key)
	case http.MethodPut:
		if r.Header.Get("x-amz-copy-source") != "" {
			s.handleCopyObject(w, r, bucket, key)
			return
		}
		s.handlePutObject(w, r, bucket, key)
	case http.MethodDelete:
		s.handleDeleteObject(w, r, bucket, key)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	}
}

func (s *Service) handleHeadObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	meta, err := s.objects.StatObject(r.Context(), bucket, key)
	if err != nil {
		s.writeMappedError(w, err)
		return
	}
	writeObjectHeaders(w, meta)
	w.WriteHeader(http.StatusOK)
}

func (s *Service) handleGetObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	body, meta, err := s.objects.GetObject(r.Context(), bucket, key)
	if err != nil {
		s.writeMappedError(w, err)
		return
	}
	s.writeRangedObject(w, r, body, meta)
}

func (s *Service) handlePutObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	meta, err := s.objects.PutObject(r.Context(), bucket, key, r.Body, putOptionsFromHeaders(r.Header))
	if err != nil {
		s.writeMappedError(w, err)
		return
	}
	writeObjectHeaders(w, meta)
	w.WriteHeader(http.StatusOK)
}

func (s *Service) handleDeleteObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if _, err := s.objects.DeleteObject(r.Context(), bucket, key); err != nil {
		s.writeMappedError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
