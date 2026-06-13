package index

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/model"
)

const maxExtractBytes = 8 << 20

func ExtractText(reader io.Reader, meta model.ObjectMeta) (string, error) {
	if reader == nil {
		return "", nil
	}
	if !supportsTextExtraction(meta) {
		return "", nil
	}
	var buf bytes.Buffer
	limited := io.LimitReader(reader, maxExtractBytes)
	if _, err := io.Copy(&buf, limited); err != nil {
		return "", fmt.Errorf("extract text: %w", err)
	}
	return normalizeExtractedText(buf.String()), nil
}

func supportsTextExtraction(meta model.ObjectMeta) bool {
	contentType := strings.ToLower(strings.TrimSpace(meta.ContentType))
	if strings.HasPrefix(contentType, "text/") {
		return true
	}
	switch contentType {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml":
		return true
	default:
		return strings.HasSuffix(strings.ToLower(meta.Key), ".md")
	}
}

func normalizeExtractedText(value string) string {
	value = strings.ReplaceAll(value, "\x00", " ")
	return strings.Join(strings.Fields(value), " ")
}
