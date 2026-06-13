package handler

import (
	"errors"
	"net/http"
)

var ErrLocalDataPlaneRemoved = errors.New("local object data plane has been removed; use the S3 proxy upstream data plane")

func (s *Service) writeS3ProxyNotImplemented(w http.ResponseWriter) {
	s.writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "s3 proxy data plane is not implemented yet"})
}

func (s *Service) writeLegacyObjectUnavailable(w http.ResponseWriter) {
	s.writeLocalDataPlaneRemoved(w)
}

func (s *Service) writeLocalDataPlaneRemoved(w http.ResponseWriter) {
	s.writeJSON(w, http.StatusNotImplemented, map[string]string{"error": ErrLocalDataPlaneRemoved.Error()})
}
