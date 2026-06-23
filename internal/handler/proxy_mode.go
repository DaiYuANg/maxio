package handler

import "net/http"

func (s *Service) writeS3ProxyDisabled(w http.ResponseWriter) {
	s.writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "s3 proxy is disabled for this control handler; set enable_s3_proxy=true to start the Vale S3 data plane on s3_proxy_entrypoint"})
}

func (s *Service) writeHandlerControlRouteNotFound(w http.ResponseWriter) {
	s.writeJSON(w, http.StatusNotFound, map[string]string{"error": "route is not served by the handler control plane; S3 data-plane traffic is handled by the configured Vale proxy entrypoint"})
}
