package processing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/samber/lo"
)

func readTikaRMeta(reader io.Reader, limit int64) (map[string]string, error) {
	data, truncated, err := readLimitedBytes(reader, limit)
	if err != nil {
		return nil, err
	}
	if truncated {
		return nil, fmt.Errorf("tika response exceeds %d bytes", limit)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]string{"document_count": "0", "text_bytes": "0", "text_truncated": "false"}, nil
	}
	documents, err := tikaDocuments(data)
	if err != nil {
		return nil, err
	}
	return summarizeTikaRMeta(documents), nil
}

func tikaDocuments(data []byte) ([]map[string]any, error) {
	documents := []map[string]any{}
	err := json.Unmarshal(data, &documents)
	if err == nil {
		return documents, nil
	}
	return tikaSingleDocument(data, err)
}

func tikaSingleDocument(data []byte, arrayErr error) ([]map[string]any, error) {
	document := map[string]any{}
	if objectErr := json.Unmarshal(data, &document); objectErr != nil {
		return nil, fmt.Errorf("unmarshal tika response: %w", errors.Join(arrayErr, objectErr))
	}
	return []map[string]any{document}, nil
}

func summarizeTikaRMeta(documents []map[string]any) map[string]string {
	metadata := map[string]string{"document_count": strconv.Itoa(len(documents))}
	var textBytes int64
	textTruncated := false
	for index, document := range documents {
		content := tikaMetadataString(document["X-TIKA:content"])
		textBytes += int64(len(content))
		if strings.EqualFold(tikaMetadataString(document["X-TIKA:Exception:write_limit_reached"]), "true") {
			textTruncated = true
		}
		if index == 0 {
			copyPrimaryTikaMetadata(metadata, document)
		}
	}
	metadata["text_bytes"] = strconv.FormatInt(textBytes, 10)
	metadata["text_truncated"] = strconv.FormatBool(textTruncated)
	return metadata
}

func copyPrimaryTikaMetadata(metadata map[string]string, document map[string]any) {
	copyTikaMetadata(metadata, document, "detected_content_type", "Content-Type")
	copyTikaMetadata(metadata, document, "content_encoding", "Content-Encoding")
	copyTikaMetadata(metadata, document, "content_length", "Content-Length")
	copyTikaMetadata(metadata, document, "resource_name", "resourceName")
	copyTikaMetadata(metadata, document, "title", "dc:title", "title")
	copyTikaMetadata(metadata, document, "author", "dc:creator", "creator", "Author")
	copyTikaMetadata(metadata, document, "language", "language", "dc:language")
	copyTikaMetadata(metadata, document, "parsed_by", "X-Parsed-By", "X-TIKA:Parsed-By")
}

func copyTikaMetadata(metadata map[string]string, document map[string]any, target string, sourceKeys ...string) {
	var selected string
	_, found := lo.Find(sourceKeys, func(sourceKey string) bool {
		selected = tikaMetadataString(document[sourceKey])
		return selected != ""
	})
	if found {
		metadata[target] = selected
	}
}

func tikaMetadataString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []string:
		return joinTikaStrings(typed)
	case []any:
		return joinTikaValues(typed)
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return tikaJSONOrString(typed)
	}
}

func joinTikaStrings(values []string) string {
	items := lo.Compact(lo.Map(values, func(value string, _ int) string {
		return strings.TrimSpace(value)
	}))
	return strings.Join(items, ",")
}

func joinTikaValues(values []any) string {
	items := lo.Compact(lo.Map(values, func(value any, _ int) string {
		return tikaMetadataString(value)
	}))
	return strings.Join(items, ",")
}

func tikaJSONOrString(value any) string {
	data, err := json.Marshal(value)
	if err == nil {
		return string(data)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
