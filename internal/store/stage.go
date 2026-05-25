package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
)

type stagedObject struct {
	file *os.File
	hash string
	size int64
}

func stageObject(reader io.Reader) (*stagedObject, error) {
	if reader == nil {
		return nil, errors.New("object reader is required")
	}
	file, err := os.CreateTemp("", "maxio-put-*")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	size, hash, err := writeStagedObject(file, reader)
	if err != nil {
		if cleanupErr := closeAndRemove(file); cleanupErr != nil {
			return nil, fmt.Errorf("%w; cleanup temp file: %w", err, cleanupErr)
		}
		return nil, err
	}
	return &stagedObject{
		file: file,
		hash: hash,
		size: size,
	}, nil
}

func writeStagedObject(file *os.File, reader io.Reader) (int64, string, error) {
	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)
	size, err := io.Copy(writer, reader)
	if err != nil {
		return 0, "", fmt.Errorf("write temp file: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", fmt.Errorf("seek staged object: %w", err)
	}
	return size, hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *stagedObject) Reader() (io.Reader, error) {
	if s == nil || s.file == nil {
		return nil, errors.New("staged object is not available")
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek staged object: %w", err)
	}
	return s.file, nil
}

func (s *stagedObject) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	return closeAndRemove(s.file)
}

func closeStagedObject(staged *stagedObject) {
	if err := staged.Close(); err != nil {
		_ = err.Error()
	}
}

func closeAndRemove(file *os.File) error {
	path := file.Name()
	err := file.Close()
	if removeErr := os.Remove(path); removeErr != nil {
		err = errors.Join(err, fmt.Errorf("remove temp file: %w", removeErr))
	}
	if err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	return nil
}
