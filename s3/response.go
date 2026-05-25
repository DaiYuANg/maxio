package s3

import (
	"encoding/xml"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/lyonbrown4d/maxio/object"
)

func (s *Service) writeXML(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", contentTypeXML)
	w.WriteHeader(status)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		s.logger.Warn("write s3 xml header failed", "error", err)
		return
	}
	if err := xml.NewEncoder(w).Encode(value); err != nil {
		s.logger.Warn("encode s3 response failed", "error", err)
	}
}

func (s *Service) writeMappedError(w http.ResponseWriter, err error) {
	status, code := mapError(err)
	s.writeError(w, status, code, err.Error())
}

func (s *Service) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeXML(w, status, errorResult{
		Code:      code,
		Message:   message,
		RequestID: requestID(),
	})
}

func mapError(err error) (int, string) {
	switch {
	case errors.Is(err, object.ErrBucketExists):
		return http.StatusConflict, "BucketAlreadyOwnedByYou"
	case errors.Is(err, object.ErrBucketNotFound):
		return http.StatusNotFound, "NoSuchBucket"
	case errors.Is(err, object.ErrNotFound):
		return http.StatusNotFound, "NoSuchKey"
	case errors.Is(err, object.ErrBadRequest):
		return http.StatusBadRequest, "InvalidArgument"
	default:
		return http.StatusInternalServerError, "InternalError"
	}
}

func writeObjectHeaders(w http.ResponseWriter, meta object.ObjectMeta) {
	w.Header().Set("ETag", meta.ETag)
	w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Last-Modified", meta.UpdatedAt.UTC().Format(http.TimeFormat))
	if meta.ContentType != "" {
		w.Header().Set("Content-Type", meta.ContentType)
	}
	writeObjectMetadataHeaders(w.Header(), meta)
}

func formatS3Time(value time.Time) string {
	if value.IsZero() {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return value.UTC().Format(time.RFC3339)
}

func requestID() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
