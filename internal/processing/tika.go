package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	collectionset "github.com/arcgolabs/collectionx/set"
)

const (
	defaultTikaURL              = "http://tika:9998"
	defaultTikaMaxBytes         = int64(100 << 20)
	defaultTikaResponseMaxBytes = int64(1 << 20)
)

type TikaConfig struct {
	URL      string
	Timeout  time.Duration
	MaxBytes int64
	FailOpen bool
}

type TikaProcessor struct {
	url      string
	timeout  time.Duration
	maxBytes int64
	failOpen bool
	client   *http.Client
}

func NewTikaProcessor(cfg TikaConfig) *TikaProcessor {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	if baseURL == "" {
		baseURL = defaultTikaURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultTikaMaxBytes
	}
	return &TikaProcessor{url: baseURL, timeout: timeout, maxBytes: maxBytes, failOpen: cfg.FailOpen, client: &http.Client{Timeout: timeout}}
}

func (p *TikaProcessor) Name() string {
	return "tika"
}

func (p *TikaProcessor) Capabilities() *collectionset.Set[Capability] {
	return collectionset.NewSet[Capability](CapabilityTextExtraction, CapabilityMetadataExtract)
}

func (p *TikaProcessor) FailOpen() bool {
	return p != nil && p.failOpen
}

func (p *TikaProcessor) Process(ctx context.Context, input Input) (ProcessorResult, error) {
	if input.OpenContent == nil {
		return p.failureResult("content stream unavailable", fmt.Errorf("%w: tika content stream unavailable", ErrProcessingFailed))
	}
	if input.Object.Size > p.maxBytes {
		return p.oversizedResult(), nil
	}
	content, err := input.OpenContent(ctx)
	if err != nil {
		return p.failureResult("open content", fmt.Errorf("open content for tika: %w", err))
	}
	defer content.Close()

	requestBody, oversized, err := p.requestBody(content, input.Object.Size)
	if err != nil {
		return p.failureResult("read request body", fmt.Errorf("read content for tika: %w", err))
	}
	if oversized {
		return p.oversizedResult(), nil
	}

	endpoint := tikaEndpoint(p.url)
	request, err := http.NewRequestWithContext(contextOrBackground(ctx), http.MethodPut, endpoint, requestBody)
	if err != nil {
		return p.failureResult("build request", fmt.Errorf("build tika request: %w", err))
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("maxEmbeddedResources", "0")
	request.Header.Set("writeLimit", strconv.FormatInt(defaultTikaResponseMaxBytes, 10))
	if input.Object.ContentType != "" {
		request.Header.Set("Content-Type", input.Object.ContentType)
	}
	if input.Object.Key != "" {
		request.Header.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": input.Object.Key}))
	}
	response, err := p.client.Do(request)
	if err != nil {
		return p.failureResult("call tika", fmt.Errorf("call tika: %w", err))
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return p.failureResult("bad status", fmt.Errorf("tika returned status %d", response.StatusCode))
	}
	metadata, err := readTikaRMeta(response.Body, defaultTikaResponseMaxBytes)
	if err != nil {
		return p.failureResult("read response", fmt.Errorf("read tika response: %w", err))
	}
	metadata["endpoint"] = endpoint
	return ProcessorResult{
		Processor: p.Name(),
		Status:    StatusSucceeded,
		Metadata:  metadata,
	}, nil
}

func (p *TikaProcessor) requestBody(content io.Reader, size int64) (io.Reader, bool, error) {
	if size > 0 {
		return content, false, nil
	}
	body, oversized, err := readTikaRequestBody(content, p.maxBytes)
	if err != nil {
		return nil, false, err
	}
	if oversized {
		return nil, true, nil
	}
	return bytes.NewReader(body), false, nil
}

func (p *TikaProcessor) oversizedResult() ProcessorResult {
	return ProcessorResult{
		Processor: p.Name(),
		Status:    StatusSkipped,
		Metadata: map[string]string{
			"reason":    "object exceeds tika max bytes",
			"max_bytes": strconv.FormatInt(p.maxBytes, 10),
		},
	}
}

func (p *TikaProcessor) failureResult(reason string, err error) (ProcessorResult, error) {
	metadata := map[string]string{
		"endpoint":  tikaEndpoint(p.url),
		"fail_open": strconv.FormatBool(p.failOpen),
		"reason":    reason,
	}
	if !p.failOpen {
		return ProcessorResult{Processor: p.Name(), Status: StatusFailed, Metadata: metadata}, err
	}
	return ProcessorResult{
		Processor: p.Name(),
		Status:    StatusSkipped,
		Error:     err.Error(),
		Metadata:  metadata,
	}, nil
}

func tikaEndpoint(baseURL string) string {
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return defaultTikaURL + "/rmeta/text"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/rmeta/text"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

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
	documents := []map[string]any{}
	if err := json.Unmarshal(data, &documents); err != nil {
		document := map[string]any{}
		if objectErr := json.Unmarshal(data, &document); objectErr != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return summarizeTikaRMeta(documents), nil
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
			copyTikaMetadata(metadata, document, "detected_content_type", "Content-Type")
			copyTikaMetadata(metadata, document, "content_encoding", "Content-Encoding")
			copyTikaMetadata(metadata, document, "content_length", "Content-Length")
			copyTikaMetadata(metadata, document, "resource_name", "resourceName")
			copyTikaMetadata(metadata, document, "title", "dc:title", "title")
			copyTikaMetadata(metadata, document, "author", "dc:creator", "creator", "Author")
			copyTikaMetadata(metadata, document, "language", "language", "dc:language")
			copyTikaMetadata(metadata, document, "parsed_by", "X-Parsed-By", "X-TIKA:Parsed-By")
		}
	}
	metadata["text_bytes"] = strconv.FormatInt(textBytes, 10)
	metadata["text_truncated"] = strconv.FormatBool(textTruncated)
	return metadata
}

func copyTikaMetadata(metadata map[string]string, document map[string]any, target string, sourceKeys ...string) {
	for _, sourceKey := range sourceKeys {
		value := tikaMetadataString(document[sourceKey])
		if value != "" {
			metadata[target] = value
			return
		}
	}
}

func tikaMetadataString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(item); value != "" {
				items = append(items, value)
			}
		}
		return strings.Join(items, ",")
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := tikaMetadataString(item); value != "" {
				items = append(items, value)
			}
		}
		return strings.Join(items, ",")
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		data, err := json.Marshal(typed)
		if err == nil {
			return string(data)
		}
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func readLimitedBytes(reader io.Reader, limit int64) ([]byte, bool, error) {
	var buffer bytes.Buffer
	count, err := io.Copy(&buffer, io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, err
	}
	if count > limit {
		return buffer.Bytes()[:limit], true, nil
	}
	return buffer.Bytes(), false, nil
}

func countLimited(reader io.Reader, limit int64) (int64, bool, error) {
	limited := io.LimitReader(reader, limit+1)
	count, err := io.Copy(io.Discard, limited)
	if err != nil {
		return 0, false, err
	}
	if count > limit {
		return limit, true, nil
	}
	return count, false, nil
}

func readTikaRequestBody(reader io.Reader, maxBytes int64) ([]byte, bool, error) {
	var buffer bytes.Buffer
	count, err := io.Copy(&buffer, io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, false, err
	}
	if count > maxBytes {
		return nil, true, nil
	}
	return buffer.Bytes(), false, nil
}
