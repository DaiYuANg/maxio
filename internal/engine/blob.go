package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/model"
)

// BlobInfo describes a stored content blob independent from object metadata.
type BlobInfo struct {
	Hash            string
	ETag            string
	Size            int64
	ShardDir        string
	ShardPlacements []model.ShardPlacement
	ShardChecksums  []string
	ShardSizes      []int64
}

func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ETagFromHash(hash string) string {
	if hash == "" {
		return ""
	}
	return `"` + hash + `"`
}

// PutBlob writes content from a reader into reusable content shards.
func (e *Engine) PutBlob(ctx context.Context, key string, reader io.Reader) (BlobInfo, error) {
	_ = ctx
	if reader == nil {
		return BlobInfo{}, errors.New("engine: reader is required")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return BlobInfo{}, fmt.Errorf("engine: read input: %w", err)
	}
	return e.PutBlobBytes(ctx, key, data)
}

// PutBlobBytes writes content shards and returns the reusable blob identity.
func (e *Engine) PutBlobBytes(ctx context.Context, key string, data []byte) (BlobInfo, error) {
	_ = ctx
	key = strings.TrimSpace(key)
	if key == "" {
		return BlobInfo{}, errors.New("engine: key is required")
	}

	hash := HashBytes(data)
	shards, err := e.coder.Encode(data)
	if err != nil {
		return BlobInfo{}, fmt.Errorf("engine: encode object: %w", err)
	}

	shardDir := makeShardDir(key)
	placements, err := e.PlanShardPlacement(ctx, PlacementRequest{
		Key:        key,
		Hash:       hash,
		ShardCount: len(shards),
	})
	if err != nil {
		return BlobInfo{}, err
	}
	checksums := shardChecksums(shards)
	sizes := shardSizes(shards)
	if err := e.writeShardSet(ctx, placements, shardDir, hash, shards); err != nil {
		return BlobInfo{}, err
	}

	return BlobInfo{
		Hash:            hash,
		ETag:            ETagFromHash(hash),
		Size:            int64(len(data)),
		ShardDir:        shardDir,
		ShardPlacements: cloneShardPlacements(placements),
		ShardChecksums:  cloneStrings(checksums),
		ShardSizes:      cloneInt64s(sizes),
	}, nil
}

func shardChecksums(shards [][]byte) []string {
	checksums := make([]string, len(shards))
	for index, shard := range shards {
		checksums[index] = HashBytes(shard)
	}
	return checksums
}

func shardSizes(shards [][]byte) []int64 {
	sizes := make([]int64, len(shards))
	for index, shard := range shards {
		sizes[index] = int64(len(shard))
	}
	return sizes
}

// DeleteBlob removes content shards for a blob once no objects reference it.
func (e *Engine) DeleteBlob(ctx context.Context, shardDir, hash string) error {
	_ = ctx
	shardDir = strings.TrimSpace(shardDir)
	hash = strings.TrimSpace(hash)
	if shardDir == "" || hash == "" {
		return errors.New("engine: shard dir and hash are required")
	}
	if err := e.backend.DeleteShardSet(shardDir, hash); err != nil {
		return fmt.Errorf("engine: delete object shards: %w", err)
	}
	return nil
}
