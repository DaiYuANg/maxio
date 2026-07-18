package processing

import (
	"bytes"
	"context"
	"errors"
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
	return p.processContent(contextOrBackground(ctx), input)
}

func (p *TikaProcessor) processContent(ctx context.Context, input Input) (result ProcessorResult, err error) {
	content, err := input.OpenContent(ctx)
	if err != nil {
		return p.failureResult("open content", fmt.Errorf("open content for tika: %w", err))
	}
	defer func() {
		err = errors.Join(err, closeTikaResources(content, nil))
	}()
	requestBody, oversized, err := p.requestBody(content, input.Object.Size)
	if err != nil {
		return p.failureResult("read request body", fmt.Errorf("read content for tika: %w", err))
	}
	if oversized {
		return p.oversizedResult(), nil
	}
	return p.call(ctx, input, requestBody)
}

func (p *TikaProcessor) call(ctx context.Context, input Input, requestBody io.Reader) (result ProcessorResult, err error) {
	endpoint := tikaEndpoint(p.url)
	request, err := p.request(ctx, endpoint, input, requestBody)
	if err != nil {
		return p.failureResult("build request", err)
	}
	response, err := p.client.Do(request)
	if err != nil {
		return p.failureResult("call tika", fmt.Errorf("call tika: %w", err))
	}
	defer func() {
		err = errors.Join(err, closeTikaResources(nil, response.Body))
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return p.failureResult("bad status", fmt.Errorf("tika returned status %d", response.StatusCode))
	}
	metadata, err := readTikaRMeta(response.Body, defaultTikaResponseMaxBytes)
	if err != nil {
		return p.failureResult("read response", fmt.Errorf("read tika response: %w", err))
	}
	metadata["endpoint"] = endpoint
	return ProcessorResult{Processor: p.Name(), Status: StatusSucceeded, Metadata: metadata}, nil
}

func (p *TikaProcessor) request(ctx context.Context, endpoint string, input Input, requestBody io.Reader) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, requestBody)
	if err != nil {
		return nil, fmt.Errorf("build tika request: %w", err)
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
	return request, nil
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
	metadata := map[string]string{"endpoint": tikaEndpoint(p.url), "fail_open": strconv.FormatBool(p.failOpen), "reason": reason}
	if !p.failOpen {
		return ProcessorResult{Processor: p.Name(), Status: StatusFailed, Metadata: metadata}, err
	}
	return ProcessorResult{Processor: p.Name(), Status: StatusSkipped, Error: err.Error(), Metadata: metadata}, nil
}

func closeTikaResources(content, responseBody io.Closer) error {
	return errors.Join(closeResourceError(content), closeResourceError(responseBody))
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
