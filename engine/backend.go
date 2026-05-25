// Package engine implements local erasure-coded object shard storage.
package engine

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// Shard represents a single erasure-coded shard.
type Shard struct {
	Index    int
	Size     int64
	Data     []byte
	Verified bool
}

func (s Shard) String() string {
	return fmt.Sprintf("shard{%d,%d}", s.Index, s.Size)
}

// shardBackend manages shard lifecycle with a backing filesystem.
type shardBackend struct {
	fs        afero.Fs
	root      string
	shardSize int64
}

func newShardBackend(fs afero.Fs, root string, shardSize int64) *shardBackend {
	if fs == nil {
		fs = afero.NewOsFs()
	}
	if root == "" {
		root = "./data/engine"
	}
	return &shardBackend{fs: fs, root: root, shardSize: shardSize}
}

func (b *shardBackend) ShardSize() int64 { return b.shardSize }

// shardPath returns the filesystem path for a given shard.
func (b *shardBackend) shardPath(shardDir, hash string, index int) string {
	return filepath.Join(b.root, shardDir, hash, fmt.Sprintf("chunk-%04d", index))
}

func (b *shardBackend) metaPath(shardDir, hash string) string {
	return filepath.Join(b.root, shardDir, hash, "meta.json")
}

// WriteShard writes a shard to disk.
func (b *shardBackend) WriteShard(shardDir, hash string, index int, data []byte) error {
	path := b.shardPath(shardDir, hash, index)
	if err := b.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("engine: create shard dir: %w", err)
	}
	file, err := b.fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("engine: open shard file: %w", err)
	}
	_, err = file.Write(data)
	if cerr := file.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("engine: write shard file: %w", err)
	}
	return nil
}

// WriteMeta writes the shard set metadata to disk.
func (b *shardBackend) WriteMeta(shardDir, hash string, data []byte) error {
	path := b.metaPath(shardDir, hash)
	if err := b.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("engine: create meta dir: %w", err)
	}
	file, err := b.fs.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("engine: open meta file: %w", err)
	}
	_, err = file.Write(data)
	if cerr := file.Close(); cerr != nil && err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("engine: write meta file: %w", err)
	}
	return nil
}

// ReadShard reads a shard from disk, returning nil if the shard is missing.
func (b *shardBackend) ReadShard(shardDir, hash string, index int) ([]byte, error) {
	path := b.shardPath(shardDir, hash, index)
	file, err := b.fs.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("engine: read shard %s-%d: %w", hash, index, err)
	}
	data, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		return nil, fmt.Errorf("engine: read shard file: %w", readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("engine: close shard file: %w", closeErr)
	}
	return data, nil
}

// ShardExists checks if a shard exists on disk.
func (b *shardBackend) ShardExists(shardDir, hash string, index int) bool {
	path := b.shardPath(shardDir, hash, index)
	_, err := b.fs.Stat(path)
	return err == nil
}

func (b *shardBackend) DeleteShard(shardDir, hash string, index int) error {
	path := b.shardPath(shardDir, hash, index)
	if err := b.fs.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("engine: delete shard: %w", err)
	}
	return nil
}

// ReadMeta reads shard set metadata from disk.
func (b *shardBackend) ReadMeta(shardDir, hash string) ([]byte, error) {
	path := b.metaPath(shardDir, hash)
	data, err := afero.ReadFile(b.fs, path)
	if err != nil {
		return nil, fmt.Errorf("engine: read meta file: %w", err)
	}
	return data, nil
}

// DeleteShardSet removes an entire shard set from disk.
func (b *shardBackend) DeleteShardSet(shardDir, hash string) error {
	dir := filepath.Join(b.root, shardDir, hash)
	if err := b.fs.RemoveAll(dir); err != nil {
		return fmt.Errorf("engine: delete shard set: %w", err)
	}
	return nil
}

// DeleteMeta removes the persisted object layout metadata.
func (b *shardBackend) DeleteMeta(shardDir, layoutID string) error {
	dir := filepath.Join(b.root, shardDir, layoutID)
	if err := b.fs.RemoveAll(dir); err != nil {
		return fmt.Errorf("engine: delete layout meta: %w", err)
	}
	return nil
}

// ListShards returns the shard indexes that exist for a shard set.
func (b *shardBackend) ListShards(shardDir, hash string) ([]int, error) {
	dir := filepath.Join(b.root, shardDir, hash)
	entries, err := afero.ReadDir(b.fs, dir)
	if err != nil {
		return nil, fmt.Errorf("engine: list shards: %w", err)
	}
	var indexes []int
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "chunk-") {
			var idx int
			if _, err := fmt.Sscanf(entry.Name(), "chunk-%d", &idx); err != nil {
				return nil, fmt.Errorf("engine: parse shard index: %w", err)
			}
			indexes = append(indexes, idx)
		}
	}
	return indexes, nil
}
