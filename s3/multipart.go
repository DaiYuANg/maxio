package s3

import (
	"errors"
	"net/http"

	"github.com/lyonbrown4d/maxio/object"
)

const multipartRootDir = "s3-multipart"

var (
	errNoSuchUpload     = errors.New("multipart upload not found")
	errInvalidUploadID  = errors.New("invalid multipart upload id")
	errInvalidPart      = errors.New("invalid multipart part")
	errInvalidPartOrder = errors.New("invalid multipart part order")
	errMalformedXML     = errors.New("malformed xml")
	errEntityTooSmall   = errors.New("multipart part is smaller than minimum allowed size")
)

func (s *Service) handleMultipartObject(w http.ResponseWriter, r *http.Request, bucket, key string) bool {
	query := r.URL.Query()
	if r.Method == http.MethodPost && hasQueryKey(query, "uploads") {
		s.handleInitiateMultipartUpload(w, r, bucket, key)
		return true
	}
	uploadID := queryValue(query, "uploadId")
	if uploadID == "" {
		return false
	}
	switch r.Method {
	case http.MethodPut:
		s.handleUploadPart(w, r, bucket, key, uploadID)
	case http.MethodGet:
		s.handleListParts(w, r, uploadID)
	case http.MethodPost:
		s.handleCompleteMultipartUpload(w, r, uploadID)
	case http.MethodDelete:
		s.handleAbortMultipartUpload(w, r, uploadID)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	}
	return true
}

func (s *Service) handleInitiateMultipartUpload(w http.ResponseWriter, r *http.Request, bucket, key string) {
	upload, err := s.multipart.initiate(r.Context(), bucket, key, putOptionsFromHeaders(r.Header))
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	s.writeXML(w, http.StatusOK, initiateMultipartUploadResult{
		XMLNS:    defaultXMLNS,
		Bucket:   upload.Bucket,
		Key:      upload.Key,
		UploadID: upload.UploadID,
	})
}

func (s *Service) handleUploadPart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string) {
	partNumber, err := parsePartNumber(r.URL.Query())
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	part, err := s.multipart.putPart(r.Context(), uploadID, bucket, key, partNumber, r.Body)
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	w.Header().Set("ETag", part.ETag)
	w.WriteHeader(http.StatusOK)
}

func (s *Service) handleListParts(w http.ResponseWriter, r *http.Request, uploadID string) {
	upload, err := s.multipart.load(r.Context(), uploadID)
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	options, err := listPartsOptionsFromQuery(r.URL.Query())
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	parts := sortedMultipartParts(upload.Parts)
	page, nextMarker, truncated := paginateMultipartParts(parts, options)
	result := listPartsResult{
		XMLNS:                defaultXMLNS,
		Bucket:               upload.Bucket,
		Key:                  upload.Key,
		UploadID:             upload.UploadID,
		PartNumberMarker:     options.PartNumberMarker,
		NextPartNumberMarker: nextMarker,
		MaxParts:             options.MaxParts,
		IsTruncated:          truncated,
		Parts:                make([]partItemResult, 0, len(page)),
	}
	for _, part := range page {
		result.Parts = append(result.Parts, partItemResult{
			PartNumber:   part.Number,
			LastModified: formatS3Time(part.UploadedAt),
			ETag:         part.ETag,
			Size:         part.Size,
		})
	}
	s.writeXML(w, http.StatusOK, result)
}

func (s *Service) handleCompleteMultipartUpload(w http.ResponseWriter, r *http.Request, uploadID string) {
	request, err := decodeCompleteMultipartUpload(r.Body)
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	assembled, upload, err := s.multipart.assemble(r.Context(), uploadID, request.Parts)
	if err != nil {
		s.writeMultipartError(w, err)
		return
	}
	_, putErr := s.objects.PutObject(r.Context(), upload.Bucket, upload.Key, assembled.file, object.PutOptions{
		ContentType:        upload.ContentType,
		CacheControl:       upload.CacheControl,
		ContentDisposition: upload.ContentDisposition,
		ContentEncoding:    upload.ContentEncoding,
		ContentLanguage:    upload.ContentLanguage,
		UserMetadata:       upload.UserMetadata,
	})
	closeErr := assembled.close()
	if putErr != nil {
		s.writeMappedError(w, putErr)
		return
	}
	if closeErr != nil {
		s.writeMultipartError(w, closeErr)
		return
	}
	if err := s.multipart.abort(r.Context(), upload.UploadID); err != nil {
		s.writeMultipartError(w, err)
		return
	}
	s.writeXML(w, http.StatusOK, completeMultipartUploadResult{
		XMLNS:    defaultXMLNS,
		Location: "/" + upload.Bucket + "/" + upload.Key,
		Bucket:   upload.Bucket,
		Key:      upload.Key,
		ETag:     assembled.etag,
	})
}

func (s *Service) handleAbortMultipartUpload(w http.ResponseWriter, r *http.Request, uploadID string) {
	if err := s.multipart.abort(r.Context(), uploadID); err != nil {
		s.writeMultipartError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) writeMultipartError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNoSuchUpload):
		s.writeError(w, http.StatusNotFound, "NoSuchUpload", err.Error())
	case errors.Is(err, errInvalidPart):
		s.writeError(w, http.StatusBadRequest, "InvalidPart", err.Error())
	case errors.Is(err, errInvalidPartOrder):
		s.writeError(w, http.StatusBadRequest, "InvalidPartOrder", err.Error())
	case errors.Is(err, errMalformedXML):
		s.writeError(w, http.StatusBadRequest, "MalformedXML", err.Error())
	case errors.Is(err, errEntityTooSmall):
		s.writeError(w, http.StatusBadRequest, "EntityTooSmall", err.Error())
	default:
		s.writeError(w, http.StatusBadRequest, "InvalidArgument", err.Error())
	}
}
