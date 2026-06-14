package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"os"

	"github.com/samber/oops"
)

const defaultPutSpoolMemoryLimit = int64(16 << 20)

type capturedRequestBody struct {
	digest   string
	size     int64
	memory   []byte
	tempPath string
}

type requestBodySpooler struct {
	buffer   bytes.Buffer
	tempFile *os.File
	tempPath string
	size     int64
}

func captureRequestBody(body io.ReadCloser) (*capturedRequestBody, error) {
	hasher := sha256.New()
	spooler := &requestBodySpooler{}
	if err := readRequestBody(body, hasher, spooler); err != nil {
		return nil, closeCaptureWithError(body, spooler, err)
	}
	if err := body.Close(); err != nil {
		return nil, closeCaptureWithError(nil, spooler, oops.Wrapf(err, "close source request body"))
	}
	if err := spooler.close(); err != nil {
		return nil, closeCaptureWithError(nil, spooler, err)
	}
	digest := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	return spooler.captured(digest), nil
}

func readRequestBody(body io.Reader, hasher io.Writer, spooler *requestBodySpooler) error {
	chunk := make([]byte, 32<<10)
	for {
		done, err := readRequestBodyChunk(body, hasher, spooler, chunk)
		if err != nil || done {
			return err
		}
	}
}

func readRequestBodyChunk(body io.Reader, hasher io.Writer, spooler *requestBodySpooler, chunk []byte) (bool, error) {
	n, readErr := body.Read(chunk)
	if n > 0 {
		if err := writeCapturedChunk(hasher, spooler, chunk[:n]); err != nil {
			return false, err
		}
	}
	return requestBodyReadDone(readErr)
}

func requestBodyReadDone(readErr error) (bool, error) {
	if readErr == nil {
		return false, nil
	}
	if errors.Is(readErr, io.EOF) {
		return true, nil
	}
	return false, oops.Wrapf(readErr, "read request body")
}

func writeCapturedChunk(hasher io.Writer, spooler *requestBodySpooler, data []byte) error {
	if _, err := hasher.Write(data); err != nil {
		return oops.Wrapf(err, "hash request body")
	}
	if err := spooler.write(data); err != nil {
		return err
	}
	return nil
}

func (s *requestBodySpooler) write(data []byte) error {
	s.size += int64(len(data))
	if s.tempFile == nil && int64(s.buffer.Len()+len(data)) <= defaultPutSpoolMemoryLimit {
		if _, err := s.buffer.Write(data); err != nil {
			return oops.Wrapf(err, "buffer request body")
		}
		return nil
	}
	if s.tempFile == nil {
		if err := s.createTempFile(); err != nil {
			return err
		}
	}
	if _, err := s.tempFile.Write(data); err != nil {
		return oops.Wrapf(err, "write request body to spool file")
	}
	return nil
}

func (s *requestBodySpooler) createTempFile() error {
	file, err := os.CreateTemp("", "maxio-s3-put-*")
	if err != nil {
		return oops.Wrapf(err, "create request body spool file")
	}
	s.tempFile = file
	s.tempPath = file.Name()
	if _, err := s.tempFile.Write(s.buffer.Bytes()); err != nil {
		return oops.Wrapf(err, "write buffered request body to spool file")
	}
	s.buffer.Reset()
	return nil
}

func (s *requestBodySpooler) close() error {
	if s.tempFile == nil {
		return nil
	}
	file := s.tempFile
	s.tempFile = nil
	if err := file.Close(); err != nil {
		return oops.Wrapf(err, "close request body spool file")
	}
	return nil
}

func (s *requestBodySpooler) cleanup() error {
	var result error
	if s.tempFile != nil {
		result = errors.Join(result, s.close())
	}
	if s.tempPath != "" {
		if err := os.Remove(s.tempPath); err != nil {
			result = errors.Join(result, oops.Wrapf(err, "remove request body spool file"))
		}
	}
	return result
}

func (s *requestBodySpooler) captured(digest string) *capturedRequestBody {
	return &capturedRequestBody{
		digest:   digest,
		size:     s.size,
		memory:   s.buffer.Bytes(),
		tempPath: s.tempPath,
	}
}

func (b *capturedRequestBody) Open() (io.ReadCloser, error) {
	if b == nil {
		err := oops.New("captured request body is nil")
		return nil, oops.Wrapf(err, "open captured request body")
	}
	if b.tempPath == "" {
		return io.NopCloser(bytes.NewReader(b.memory)), nil
	}
	file, err := os.Open(b.tempPath)
	if err != nil {
		return nil, oops.Wrapf(err, "open request body spool file")
	}
	return file, nil
}

func closeCaptureWithError(body io.Closer, spooler *requestBodySpooler, cause error) error {
	var result error
	result = errors.Join(result, cause)
	if body != nil {
		if err := body.Close(); err != nil {
			result = errors.Join(result, oops.Wrapf(err, "close source request body"))
		}
	}
	if spooler != nil {
		result = errors.Join(result, spooler.cleanup())
	}
	return result
}

func cleanupCapturedBody(ctx context.Context, logger *slog.Logger, body *capturedRequestBody) {
	if body == nil || body.tempPath == "" {
		return
	}
	if err := os.Remove(body.tempPath); err != nil && logger != nil {
		logger.WarnContext(ctx, "remove s3 put request body spool file", "path", body.tempPath, "error", err)
	}
}

func closeReplayedPutBody(ctx context.Context, logger *slog.Logger, body io.Closer) {
	if body == nil {
		return
	}
	if err := body.Close(); err != nil && logger != nil {
		logger.DebugContext(ctx, "close replayed s3 put request body", "error", err)
	}
}
