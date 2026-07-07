package processing

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	collectionset "github.com/arcgolabs/collectionx/set"
)

const (
	defaultClamAVAddress          = "clamav:3310"
	defaultClamAVResponseMaxBytes = int64(4 << 10)
)

type ClamAVConfig struct {
	Address string
	Timeout time.Duration
}

type ClamAVProcessor struct {
	address string
	timeout time.Duration
}

func NewClamAVProcessor(cfg ClamAVConfig) *ClamAVProcessor {
	address := strings.TrimSpace(cfg.Address)
	if address == "" {
		address = defaultClamAVAddress
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &ClamAVProcessor{address: address, timeout: timeout}
}

func (p *ClamAVProcessor) Name() string {
	return "clamav"
}

func (p *ClamAVProcessor) Capabilities() *collectionset.Set[Capability] {
	return collectionset.NewSet[Capability](CapabilityAntivirus, CapabilityPolicyEvaluation)
}

func (p *ClamAVProcessor) Process(ctx context.Context, input Input) (ProcessorResult, error) {
	if input.OpenContent == nil {
		return p.failureResult("content stream unavailable", fmt.Errorf("%w: clamav content stream unavailable", ErrProcessingFailed))
	}
	ctx, cancel := context.WithTimeout(contextOrBackground(ctx), p.timeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", p.address)
	if err != nil {
		return p.failureResult("connect", fmt.Errorf("connect clamav: %w", err))
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return p.failureResult("set deadline", fmt.Errorf("set clamav deadline: %w", err))
		}
	}

	content, err := input.OpenContent(ctx)
	if err != nil {
		return p.failureResult("open content", fmt.Errorf("open content for clamav: %w", err))
	}
	defer content.Close()

	if err := writeFull(conn, []byte("zINSTREAM\x00")); err != nil {
		return p.failureResult("start instream", fmt.Errorf("start clamav instream: %w", err))
	}
	if err := writeClamAVStream(conn, content); err != nil {
		return p.failureResult("stream content", err)
	}
	response, err := readClamAVResponse(conn)
	if err != nil {
		return p.failureResult("read response", err)
	}
	return p.resultFromResponse(response)
}

func (p *ClamAVProcessor) failureResult(reason string, err error) (ProcessorResult, error) {
	return ProcessorResult{
		Processor: p.Name(),
		Status:    StatusFailed,
		Metadata: map[string]string{
			"address": p.address,
			"reason":  reason,
		},
	}, err
}
func writeClamAVStream(writer io.Writer, reader io.Reader) error {
	chunk := make([]byte, 32<<10)
	for {
		n, readErr := reader.Read(chunk)
		if n > 0 {
			if err := writeClamAVChunk(writer, chunk[:n]); err != nil {
				return err
			}
		}
		if readErr == nil {
			continue
		}
		if readErr == io.EOF {
			return writeClamAVChunk(writer, nil)
		}
		return fmt.Errorf("read content for clamav: %w", readErr)
	}
}

func writeClamAVChunk(writer io.Writer, chunk []byte) error {
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(chunk)))
	if err := writeFull(writer, header); err != nil {
		return fmt.Errorf("write clamav chunk header: %w", err)
	}
	if len(chunk) == 0 {
		return nil
	}
	if err := writeFull(writer, chunk); err != nil {
		return fmt.Errorf("write clamav chunk body: %w", err)
	}
	return nil
}

func writeFull(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func readClamAVResponse(reader io.Reader) (string, error) {
	response, err := bufio.NewReader(io.LimitReader(reader, defaultClamAVResponseMaxBytes+1)).ReadString(0)
	if int64(len(response)) > defaultClamAVResponseMaxBytes {
		return "", fmt.Errorf("clamav response exceeds %d bytes", defaultClamAVResponseMaxBytes)
	}
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read clamav response: %w", err)
	}
	return strings.TrimSpace(strings.TrimRight(response, "\x00")), nil
}

func (p *ClamAVProcessor) resultFromResponse(response string) (ProcessorResult, error) {
	metadata := clamAVResponseMetadata(response)
	switch metadata["verdict"] {
	case "infected":
		return ProcessorResult{Processor: p.Name(), Status: StatusBlocked, Metadata: metadata}, fmt.Errorf("%w: %s", ErrProcessingDenied, response)
	case "clean":
		return ProcessorResult{Processor: p.Name(), Status: StatusSucceeded, Metadata: metadata}, nil
	default:
		return ProcessorResult{Processor: p.Name(), Status: StatusFailed, Metadata: metadata}, fmt.Errorf("unexpected clamav response: %s", response)
	}
}

func clamAVResponseMetadata(response string) map[string]string {
	metadata := map[string]string{"response": response}
	trimmed := strings.TrimSpace(response)
	upper := strings.ToUpper(trimmed)
	if upper == "OK" || strings.HasSuffix(upper, " OK") {
		metadata["verdict"] = "clean"
		return metadata
	}
	if upper == "FOUND" || strings.HasSuffix(upper, " FOUND") {
		metadata["verdict"] = "infected"
		if signature := clamAVSignature(trimmed); signature != "" {
			metadata["signature"] = signature
		}
		return metadata
	}
	metadata["verdict"] = "unknown"
	return metadata
}

func clamAVSignature(response string) string {
	upper := strings.ToUpper(response)
	index := strings.LastIndex(upper, " FOUND")
	if index < 0 {
		return ""
	}
	signature := strings.TrimSpace(response[:index])
	if colon := strings.LastIndex(signature, ":"); colon >= 0 {
		signature = strings.TrimSpace(signature[colon+1:])
	}
	return signature
}
