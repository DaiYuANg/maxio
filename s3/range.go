package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/arcgolabs/httpx"
	"github.com/lyonbrown4d/maxio/internal/bytesrange"
	"github.com/lyonbrown4d/maxio/object"
)

func (s *Service) writeRangedObject(
	w http.ResponseWriter,
	r *http.Request,
	body io.ReadCloser,
	meta object.ObjectMeta,
) {
	reqCtx := r.Context()
	defer closeS3Body(reqCtx, s, body)

	spec, err := bytesrange.Parse(r.Header.Get("Range"), meta.Size)
	if err != nil {
		w.Header().Set("Content-Range", bytesrange.UnsatisfiedContentRange(meta.Size))
		s.writeError(w, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", err.Error())
		return
	}
	data, err := io.ReadAll(body)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	writeRangedObjectHeaders(w, meta, spec)
	if spec.Partial {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if _, err := io.Copy(w, bytes.NewReader(spec.Slice(data))); err != nil {
		s.logger.WarnContext(reqCtx, "write s3 object body failed", "error", err)
	}
}

func (s *Service) rangedObjectHTTPX(
	ctx context.Context,
	rangeHeader string,
	body io.ReadCloser,
	meta object.ObjectMeta,
) (*httpxOutput, error) {
	defer closeS3Body(ctx, s, body)

	spec, err := bytesrange.Parse(rangeHeader, meta.Size)
	if err != nil {
		return s.errorHTTPX(http.StatusRequestedRangeNotSatisfiable, "InvalidRange", err.Error())
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return nil, errors.Join(object.ErrEngineFailed, err)
	}
	out := s.objectHeadersHTTPX(http.StatusOK, meta)
	if spec.Partial {
		out.Status = http.StatusPartialContent
		out.ContentRange = spec.ContentRange()
	}
	out.ContentLength = strconv.FormatInt(spec.ContentLength(), 10)
	out.Body = httpx.StreamBytes(spec.Slice(data))
	return out, nil
}

func closeS3Body(ctx context.Context, s *Service, body io.Closer) {
	if closeErr := body.Close(); closeErr != nil {
		s.logger.WarnContext(ctx, "close s3 object body failed", "error", closeErr)
	}
}

func writeRangedObjectHeaders(w http.ResponseWriter, meta object.ObjectMeta, spec bytesrange.Spec) {
	writeObjectHeaders(w, meta)
	w.Header().Set("Content-Length", strconv.FormatInt(spec.ContentLength(), 10))
	if spec.Partial {
		w.Header().Set("Content-Range", spec.ContentRange())
	}
}
