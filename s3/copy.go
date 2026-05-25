package s3

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strings"

	"github.com/lyonbrown4d/maxio/object"
)

const (
	copySourceHeader        = "x-amz-copy-source"
	metadataDirectiveHeader = "x-amz-metadata-directive"
)

func (s *Service) handleCopyObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	sourceBucket, sourceKey, err := parseCopySource(r.Header.Get(copySourceHeader))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "InvalidArgument", err.Error())
		return
	}
	body, sourceMeta, err := s.objects.GetObject(r.Context(), sourceBucket, sourceKey)
	if err != nil {
		s.writeMappedError(w, err)
		return
	}
	defer closeS3Body(r.Context(), s, body)

	opts, err := copyObjectPutOptions(sourceMeta, r.Header)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "InvalidArgument", err.Error())
		return
	}
	meta, err := s.objects.PutObject(r.Context(), bucket, key, body, opts)
	if err != nil {
		s.writeMappedError(w, err)
		return
	}
	w.Header().Set("ETag", meta.ETag)
	s.writeXML(w, http.StatusOK, copyObjectResult{
		LastModified: formatS3Time(meta.UpdatedAt),
		ETag:         meta.ETag,
	})
}

func parseCopySource(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", fmt.Errorf("%s header is required", copySourceHeader)
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.EscapedPath()
	} else if before, _, ok := strings.Cut(value, "?"); ok {
		value = before
	}
	bucket, key, err := splitS3Path(value)
	if err != nil {
		return "", "", err
	}
	if bucket == "" || key == "" {
		return "", "", errors.New("copy source must include bucket and key")
	}
	return bucket, key, nil
}

func copyObjectPutOptions(source object.ObjectMeta, headers http.Header) (object.PutOptions, error) {
	directive := strings.ToUpper(strings.TrimSpace(headers.Get(metadataDirectiveHeader)))
	switch directive {
	case "", "COPY":
		return object.PutOptions{
			ContentType:        source.ContentType,
			CacheControl:       source.CacheControl,
			ContentDisposition: source.ContentDisposition,
			ContentEncoding:    source.ContentEncoding,
			ContentLanguage:    source.ContentLanguage,
			UserMetadata:       cloneS3UserMetadata(source.UserMetadata),
		}, nil
	case "REPLACE":
		return putOptionsFromHeaders(headers), nil
	default:
		return object.PutOptions{}, fmt.Errorf("unsupported metadata directive %q", directive)
	}
}

func cloneS3UserMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	maps.Copy(output, input)
	return output
}
