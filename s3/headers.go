package s3

import (
	"net/http"
	"sort"
	"strings"

	"github.com/lyonbrown4d/maxio/object"
)

const userMetadataHeaderPrefix = "x-amz-meta-"

func putOptionsFromHeaders(headers http.Header) object.PutOptions {
	return object.PutOptions{
		ContentType:        headers.Get("Content-Type"),
		CacheControl:       headers.Get("Cache-Control"),
		ContentDisposition: headers.Get("Content-Disposition"),
		ContentEncoding:    headers.Get("Content-Encoding"),
		ContentLanguage:    headers.Get("Content-Language"),
		UserMetadata:       userMetadataFromHeaders(headers),
	}
}

func userMetadataFromHeaders(headers http.Header) map[string]string {
	metadata := make(map[string]string)
	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		if !strings.HasPrefix(lowerKey, userMetadataHeaderPrefix) {
			continue
		}
		name := strings.TrimSpace(strings.TrimPrefix(lowerKey, userMetadataHeaderPrefix))
		value := strings.TrimSpace(strings.Join(values, ","))
		if name == "" || value == "" {
			continue
		}
		metadata[name] = value
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func writeObjectMetadataHeaders(headers http.Header, meta object.ObjectMeta) {
	setHeaderIfNotEmpty(headers, "Cache-Control", meta.CacheControl)
	setHeaderIfNotEmpty(headers, "Content-Disposition", meta.ContentDisposition)
	setHeaderIfNotEmpty(headers, "Content-Encoding", meta.ContentEncoding)
	setHeaderIfNotEmpty(headers, "Content-Language", meta.ContentLanguage)
	keys := make([]string, 0, len(meta.UserMetadata))
	for key := range meta.UserMetadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(meta.UserMetadata[key])
		if strings.TrimSpace(key) == "" || value == "" {
			continue
		}
		headers.Set(userMetadataHeaderPrefix+strings.ToLower(key), value)
	}
}

func setHeaderIfNotEmpty(headers http.Header, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	headers.Set(key, value)
}
