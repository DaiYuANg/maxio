package proxy

import "net/http"

func (m *dedupeMiddleware) handleDelete(next http.Handler, w http.ResponseWriter, r *http.Request) {
	bucket, key, ok := parseS3ObjectPath(r.URL.Path)
	if !ok {
		next.ServeHTTP(w, r)
		return
	}
	version, found, err := m.currentObjectVersion(r.Context(), bucket, key)
	if err != nil {
		writeS3ProxyInternalError(w, "failed to lookup object metadata")
		m.logger.ErrorContext(r.Context(), "lookup s3 delete object metadata", "bucket", bucket, "key", key, "error", err)
		return
	}
	if !found {
		next.ServeHTTP(w, r)
		return
	}
	if _, err := m.store.DeleteObjectRecord(r.Context(), bucket, key); err != nil {
		writeS3ProxyInternalError(w, "failed to delete object metadata")
		m.logger.ErrorContext(r.Context(), "delete s3 object record", "bucket", bucket, "key", key, "error", err)
		return
	}
	m.deleteCachedObjectVersion(r.Context(), bucket, key)
	if err := m.releaseDigest(r.Context(), version.Digest); err != nil {
		writeS3ProxyInternalError(w, "failed to release object digest")
		m.logger.ErrorContext(r.Context(), "release s3 object digest", "bucket", bucket, "key", key, "digest", version.Digest, "error", err)
		return
	}
	m.discardProcessingRecord(r.Context(), version)
	w.Header().Set("X-Maxio-Dedupe", "delete-ref")
	w.WriteHeader(http.StatusNoContent)
	m.publishObjectDelete(r.Context(), ObjectDeleteSucceededEvent{Bucket: bucket, Key: key, Digest: digest, Deleted: true})
}

func (m *dedupeMiddleware) handleRead(next http.Handler, upstreamID string, w http.ResponseWriter, r *http.Request) {
	bucket, key, ok := parseS3ObjectPath(r.URL.Path)
	if !ok {
		next.ServeHTTP(w, r)
		return
	}
	version, found, err := m.currentObjectVersion(r.Context(), bucket, key)
	if err != nil {
		writeS3ProxyInternalError(w, "failed to lookup object metadata")
		m.logger.ErrorContext(r.Context(), "lookup s3 read object metadata", "bucket", bucket, "key", key, "error", err)
		return
	}
	if !found || version.UpstreamBucket == "" || version.UpstreamKey == "" {
		next.ServeHTTP(w, r)
		return
	}
	if !m.ensureProcessingReadAllowed(r.Context(), w, version) {
		return
	}
	if version.UpstreamID != "" && version.UpstreamID != upstreamID {
		m.logger.WarnContext(
			r.Context(),
			"skip cross-upstream dedupe read rewrite",
			"bucket", bucket,
			"key", key,
			"route_upstream", upstreamID,
			"canonical_upstream", version.UpstreamID,
		)
		next.ServeHTTP(w, r)
		return
	}
	r.URL.Path = s3ObjectPath(version.UpstreamBucket, version.UpstreamKey)
	next.ServeHTTP(w, r)
}
