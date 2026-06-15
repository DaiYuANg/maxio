package handler

import "net/http"

func (s *Service) writeS3ProxyNotImplemented(w http.ResponseWriter) {
	s.writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "s3 proxy data plane is not implemented yet"})
}
