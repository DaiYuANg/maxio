package s3

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
)

func splitS3Path(rawPath string) (string, string, error) {
	rawPath = strings.TrimPrefix(rawPath, compatPrefix)
	cleaned := strings.Trim(path.Clean("/"+rawPath), "/")
	if cleaned == "" {
		return "", "", nil
	}
	parts := strings.SplitN(cleaned, "/", 2)
	bucket, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", fmt.Errorf("invalid bucket path: %w", err)
	}
	if len(parts) == 1 {
		return bucket, "", nil
	}
	key, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("invalid object key path: %w", err)
	}
	return bucket, key, nil
}

func isCompatPrefix(rawPath string) bool {
	return rawPath == compatPrefix || strings.HasPrefix(rawPath, compatPrefix+"/")
}

func isReservedNativePath(rawPath string) bool {
	cleaned := strings.Trim(path.Clean("/"+rawPath), "/")
	return strings.HasPrefix(cleaned, "_") || cleaned == "health" || cleaned == "healthz"
}

func hasS3Query(query url.Values) bool {
	for key := range query {
		switch strings.ToLower(key) {
		case "list-type", "location", "uploads", "uploadid", "partnumber", "versionid", "versions", "delete":
			return true
		}
	}
	return false
}

func hasS3Header(header http.Header) bool {
	for key := range header {
		if strings.HasPrefix(strings.ToLower(key), "x-amz-") {
			return true
		}
	}
	return false
}
