package handler

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/lyonbrown4d/maxio/internal/bytesrange"
	"github.com/lyonbrown4d/maxio/internal/object"
)

func (s *Service) writeGetObjectResponse(
	w http.ResponseWriter,
	r *http.Request,
	body io.ReadCloser,
	meta object.ObjectMeta,
) {
	reqCtx := r.Context()
	defer func() {
		if closeErr := body.Close(); closeErr != nil {
			s.logger.WarnContext(reqCtx, "close object body failed", "error", closeErr)
		}
	}()

	spec, err := bytesrange.Parse(r.Header.Get("Range"), meta.Size)
	if err != nil {
		w.Header().Set("Content-Range", bytesrange.UnsatisfiedContentRange(meta.Size))
		s.writeJSON(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": err.Error()})
		return
	}
	data, err := io.ReadAll(body)
	if err != nil {
		s.writeError(w, errors.Join(object.ErrEngineFailed, err))
		return
	}
	writeRangedObjectHeaders(w, meta, spec)
	if spec.Partial {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if _, copyErr := io.Copy(w, bytes.NewReader(spec.Slice(data))); copyErr != nil {
		s.logger.WarnContext(reqCtx, "write object body failed", "error", copyErr)
	}
}

func writeRangedObjectHeaders(w http.ResponseWriter, meta object.ObjectMeta, spec bytesrange.Spec) {
	writeObjectHeaders(w, meta)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", formatInt(spec.ContentLength()))
	if spec.Partial {
		w.Header().Set("Content-Range", spec.ContentRange())
	}
}
