package s3

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lyonbrown4d/maxio/object"
	"github.com/spf13/afero"
)

func newMultipartStore(cfg Config) *multipartStore {
	dataDir := strings.TrimSpace(cfg.DataDir)
	if dataDir == "" {
		dataDir = "./data"
	}
	return &multipartStore{root: filepath.Join(dataDir, multipartRootDir)}
}

func (m *multipartStore) initiate(ctx context.Context, bucket, key string, opts object.PutOptions) (multipartUpload, error) {
	if err := contextError(ctx, "initiate multipart upload"); err != nil {
		return multipartUpload{}, err
	}
	upload := multipartUpload{
		UploadID:           newUploadID(),
		Bucket:             strings.TrimSpace(bucket),
		Key:                strings.TrimSpace(key),
		ContentType:        strings.TrimSpace(opts.ContentType),
		CacheControl:       strings.TrimSpace(opts.CacheControl),
		ContentDisposition: strings.TrimSpace(opts.ContentDisposition),
		ContentEncoding:    strings.TrimSpace(opts.ContentEncoding),
		ContentLanguage:    strings.TrimSpace(opts.ContentLanguage),
		UserMetadata:       cloneMultipartUserMetadata(opts.UserMetadata),
		CreatedAt:          time.Now().UTC(),
		Parts:              make(map[int]multipartPart),
	}
	if upload.Bucket == "" || upload.Key == "" {
		return multipartUpload{}, errInvalidPart
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.MkdirAll(m.partsDir(upload.UploadID), 0o750); err != nil {
		return multipartUpload{}, fmt.Errorf("create multipart upload: %w", err)
	}
	if err := m.saveLocked(upload); err != nil {
		return multipartUpload{}, err
	}
	return upload, nil
}

func (m *multipartStore) putPart(
	ctx context.Context,
	uploadID string,
	bucket string,
	key string,
	partNumber int,
	reader io.Reader,
) (multipartPart, error) {
	if err := contextError(ctx, "put multipart part"); err != nil {
		return multipartPart{}, err
	}
	if reader == nil {
		return multipartPart{}, errInvalidPart
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	upload, err := m.loadLocked(uploadID)
	if err != nil {
		return multipartPart{}, err
	}
	if upload.Bucket != bucket || upload.Key != key {
		return multipartPart{}, errNoSuchUpload
	}
	part, err := m.writePartLocked(upload.UploadID, partNumber, reader)
	if err != nil {
		return multipartPart{}, err
	}
	upload.Parts[partNumber] = part
	if err := m.saveLocked(upload); err != nil {
		return multipartPart{}, err
	}
	return part, nil
}

func (m *multipartStore) load(ctx context.Context, uploadID string) (multipartUpload, error) {
	if err := contextError(ctx, "load multipart upload"); err != nil {
		return multipartUpload{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadLocked(uploadID)
}

func (m *multipartStore) assemble(
	ctx context.Context,
	uploadID string,
	requestParts []completeMultipartPart,
) (assembledMultipart, multipartUpload, error) {
	if err := contextError(ctx, "assemble multipart upload"); err != nil {
		return assembledMultipart{}, multipartUpload{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	upload, err := m.loadLocked(uploadID)
	if err != nil {
		return assembledMultipart{}, multipartUpload{}, err
	}
	parts, err := completeParts(upload, requestParts)
	if err != nil {
		return assembledMultipart{}, multipartUpload{}, err
	}
	assembled, err := m.assembleLocked(upload.UploadID, parts)
	if err != nil {
		return assembledMultipart{}, multipartUpload{}, err
	}
	return assembled, upload, nil
}

func (m *multipartStore) abort(ctx context.Context, uploadID string) error {
	if err := contextError(ctx, "abort multipart upload"); err != nil {
		return err
	}
	if err := validateUploadID(uploadID); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := os.Stat(m.metadataPath(uploadID)); err != nil {
		if os.IsNotExist(err) {
			return errNoSuchUpload
		}
		return fmt.Errorf("stat multipart upload: %w", err)
	}
	if err := os.RemoveAll(m.uploadDir(uploadID)); err != nil {
		return fmt.Errorf("remove multipart upload: %w", err)
	}
	return nil
}

func (m *multipartStore) loadLocked(uploadID string) (multipartUpload, error) {
	if err := validateUploadID(uploadID); err != nil {
		return multipartUpload{}, err
	}
	file, err := os.Open(m.metadataPath(uploadID))
	if err != nil {
		if os.IsNotExist(err) {
			return multipartUpload{}, errNoSuchUpload
		}
		return multipartUpload{}, fmt.Errorf("open multipart metadata: %w", err)
	}
	defer closeFile(file)

	upload := multipartUpload{}
	if err := json.NewDecoder(file).Decode(&upload); err != nil {
		return multipartUpload{}, fmt.Errorf("decode multipart metadata: %w", err)
	}
	if upload.Parts == nil {
		upload.Parts = make(map[int]multipartPart)
	}
	return upload, nil
}

func (m *multipartStore) saveLocked(upload multipartUpload) error {
	if err := os.MkdirAll(m.uploadDir(upload.UploadID), 0o750); err != nil {
		return fmt.Errorf("create multipart metadata dir: %w", err)
	}
	data, err := json.MarshalIndent(upload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode multipart metadata: %w", err)
	}
	tempPath := m.metadataPath(upload.UploadID) + ".tmp"
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("write multipart metadata: %w", err)
	}
	if err := os.Rename(tempPath, m.metadataPath(upload.UploadID)); err != nil {
		return fmt.Errorf("commit multipart metadata: %w", err)
	}
	return nil
}

func (m *multipartStore) writePartLocked(uploadID string, partNumber int, reader io.Reader) (multipartPart, error) {
	if partNumber < 1 || partNumber > 10000 {
		return multipartPart{}, errInvalidPart
	}
	tempFile, err := os.CreateTemp(m.partsDir(uploadID), "part-*")
	if err != nil {
		return multipartPart{}, fmt.Errorf("create multipart part: %w", err)
	}
	hasher := newMultipartPartHasher()
	size, copyErr := io.Copy(io.MultiWriter(tempFile, hasher), reader)
	closeErr := tempFile.Close()
	if copyErr != nil {
		return multipartPart{}, fmt.Errorf("write multipart part: %w", copyErr)
	}
	if closeErr != nil {
		return multipartPart{}, fmt.Errorf("close multipart part: %w", closeErr)
	}
	if err := os.Rename(tempFile.Name(), m.partPath(uploadID, partNumber)); err != nil {
		return multipartPart{}, fmt.Errorf("commit multipart part: %w", err)
	}
	return multipartPart{
		Number:     partNumber,
		ETag:       quoteETag(hasher.ETag()),
		Digest:     hasher.Digest(),
		Size:       size,
		UploadedAt: time.Now().UTC(),
	}, nil
}

func (m *multipartStore) assembleLocked(uploadID string, parts []multipartPart) (assembledMultipart, error) {
	output, err := os.CreateTemp(m.uploadDir(uploadID), "complete-*")
	if err != nil {
		return assembledMultipart{}, fmt.Errorf("create multipart assembly: %w", err)
	}
	for _, part := range parts {
		if err := copyPartFile(output, m.partPath(uploadID, part.Number)); err != nil {
			return assembledMultipart{}, closeAssembleOnError(output, err)
		}
	}
	if _, err := output.Seek(0, io.SeekStart); err != nil {
		return assembledMultipart{}, closeAssembleOnError(output, fmt.Errorf("seek multipart assembly: %w", err))
	}
	return assembledMultipart{file: output, etag: multipartCompleteETag(parts)}, nil
}

func copyPartFile(output io.Writer, sourcePath string) error {
	input, err := afero.NewOsFs().Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open multipart part: %w", err)
	}
	defer closeAferoFile(input)
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy multipart part: %w", err)
	}
	return nil
}

func (m *multipartStore) uploadDir(uploadID string) string {
	return filepath.Join(m.root, uploadID)
}

func (m *multipartStore) partsDir(uploadID string) string {
	return filepath.Join(m.uploadDir(uploadID), "parts")
}

func (m *multipartStore) metadataPath(uploadID string) string {
	return filepath.Join(m.uploadDir(uploadID), "metadata.json")
}

func (m *multipartStore) partPath(uploadID string, partNumber int) string {
	return filepath.Join(m.partsDir(uploadID), fmt.Sprintf("%05d.part", partNumber))
}

func newUploadID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(data[:])
}

func closeAssembleOnError(file *os.File, cause error) error {
	if closeErr := file.Close(); closeErr != nil {
		return errors.Join(cause, fmt.Errorf("close multipart assembly: %w", closeErr))
	}
	return cause
}
