package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/afero"
)

func (e *Engine) ListObjects(ctx context.Context, bucket string) ([]ObjectInfo, error) {
	_ = ctx
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, errors.New("engine: bucket is required")
	}
	return e.scanObjectInfos(bucket)
}

func (e *Engine) scanObjectInfos(bucket string) ([]ObjectInfo, error) {
	var results []ObjectInfo
	walkFn := func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || info.Name() != "meta.json" {
			return nil
		}
		objectInfo, ok, err := e.readObjectInfo(path, bucket)
		if err != nil {
			return err
		}
		if ok {
			results = append(results, objectInfo)
		}
		return nil
	}
	if err := afero.Walk(e.fs, e.root, walkFn); err != nil {
		return nil, fmt.Errorf("engine: scan object layouts: %w", err)
	}
	return results, nil
}

func (e *Engine) readObjectInfo(path, bucket string) (ObjectInfo, bool, error) {
	data, err := afero.ReadFile(e.fs, path)
	if err != nil {
		return ObjectInfo{}, false, fmt.Errorf("engine: read layout %s: %w", path, err)
	}
	var layout Layout
	if err := json.Unmarshal(data, &layout); err != nil {
		return ObjectInfo{}, false, fmt.Errorf("engine: unmarshal layout %s: %w", path, err)
	}
	if layout.Bucket != bucket {
		return ObjectInfo{}, false, nil
	}
	return e.objectInfoFromLayout(&layout), true, nil
}

// getLayout retrieves layout from cache or disk.
func (e *Engine) getLayout(bucket, key string) (*Layout, error) {
	lk := layoutKey(bucket, key)

	// Try cache first
	if cached, ok := e.layoutCache.Load(lk); ok {
		layout, ok := cached.(*Layout)
		if ok {
			return layout, nil
		}
		e.layoutCache.Delete(lk)
	}

	// Compute shard dir and hash
	shardDir := makeShardDir(key)
	layoutID := layoutHash(lk)

	// Try to read from disk
	metaBytes, err := e.backend.ReadMeta(shardDir, layoutID)
	if err != nil {
		layout, scanErr := e.scanObjectLayout(bucket, key)
		if scanErr != nil {
			return nil, ErrObjectNotFound
		}
		e.layoutCache.Store(lk, layout)
		return layout, nil
	}

	var layout Layout
	if err := json.Unmarshal(metaBytes, &layout); err != nil {
		return nil, fmt.Errorf("engine: unmarshal layout: %w", err)
	}
	if layout.ID == "" {
		layout.ID = layoutID
	}
	if layout.ShardDir == "" {
		layout.ShardDir = shardDir
	}

	// Cache it
	e.layoutCache.Store(lk, &layout)
	return &layout, nil
}

func (e *Engine) scanObjectLayout(bucket, key string) (*Layout, error) {
	scanner := objectLayoutScanner{
		engine: e,
		bucket: bucket,
		key:    key,
	}
	if err := afero.Walk(e.fs, e.root, scanner.visit); err != nil {
		return nil, fmt.Errorf("engine: scan object layout: %w", err)
	}
	if scanner.found == nil {
		return nil, ErrObjectNotFound
	}
	return scanner.found, nil
}

type objectLayoutScanner struct {
	engine *Engine
	bucket string
	key    string
	found  *Layout
}

func (s *objectLayoutScanner) visit(path string, info os.FileInfo, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if !s.shouldRead(info) {
		return nil
	}
	layout, err := s.read(path)
	if err != nil {
		return err
	}
	if layout.Bucket == s.bucket && layout.Key == s.key {
		s.normalize(layout)
		s.found = layout
	}
	return nil
}

func (s *objectLayoutScanner) shouldRead(info os.FileInfo) bool {
	return s.found == nil && !info.IsDir() && info.Name() == "meta.json"
}

func (s *objectLayoutScanner) read(path string) (*Layout, error) {
	data, err := afero.ReadFile(s.engine.fs, path)
	if err != nil {
		return nil, fmt.Errorf("engine: read layout %s: %w", path, err)
	}
	var layout Layout
	if err := json.Unmarshal(data, &layout); err != nil {
		return nil, fmt.Errorf("engine: unmarshal layout %s: %w", path, err)
	}
	return &layout, nil
}

func (s *objectLayoutScanner) normalize(layout *Layout) {
	if layout.ID == "" {
		layout.ID = layoutHash(layoutKey(s.bucket, s.key))
	}
}

func (e *Engine) canRebuild(ctx context.Context, layout *Layout) bool {
	available := e.countReadableShards(ctx, layout)
	return available >= e.dataChunks
}

func (e *Engine) rebuildShards(ctx context.Context, layout *Layout) error {
	shards, _, err := e.readShardSetForRecovery(ctx, layout)
	if err != nil {
		return fmt.Errorf("engine: read shards for rebuild: %w", err)
	}

	// Use coder to rebuild missing parity shards
	// We only need to rebuild parity shards, data shards are already available
	if err := e.coder.Rebuild(shards); err != nil {
		return fmt.Errorf("engine: rebuild shards: %w", err)
	}

	// Write rebuilt shards
	placements := e.shardPlacements(layout)
	if err := e.writeShardSet(ctx, placements, layout.ShardDir, layout.Hash, shards); err != nil {
		return fmt.Errorf("engine: write rebuilt shards: %w", err)
	}
	return nil
}

func (e *Engine) readAvailableShards(ctx context.Context, layout *Layout) ([][]byte, int, error) {
	return e.readShardSetForRecovery(ctx, layout)
}

func (e *Engine) ensureReadableShards(ctx context.Context, layout *Layout, shards [][]byte, available int) error {
	if available >= e.dataChunks {
		return nil
	}
	if !e.canRebuild(ctx, layout) {
		return ErrShardRecoveryFailed
	}
	if err := e.rebuildShards(ctx, layout); err != nil {
		return fmt.Errorf("%w: %w", ErrShardRecoveryFailed, err)
	}
	available, err := e.fillMissingShards(ctx, layout, shards)
	if err != nil {
		return err
	}
	if available < e.dataChunks {
		return ErrShardRecoveryFailed
	}
	return nil
}

func (e *Engine) fillMissingShards(ctx context.Context, layout *Layout, shards [][]byte) (int, error) {
	refreshed, _, err := e.readShardSetForRecovery(ctx, layout)
	if err != nil {
		return 0, fmt.Errorf("engine: re-read shards: %w", err)
	}

	available := 0
	for i := range shards {
		if shards[i] == nil {
			shards[i] = refreshed[i]
		}
		if shards[i] != nil {
			available++
		}
	}
	return available, nil
}

func (e *Engine) objectInfoFromLayout(layout *Layout) ObjectInfo {
	updatedAt := layout.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return ObjectInfo{
		ObjectMeta: ObjectMeta{
			Bucket:          layout.Bucket,
			Key:             layout.Key,
			Hash:            layout.Hash,
			ETag:            layout.ETag,
			Size:            layout.Size,
			ContentType:     layout.ContentType,
			UpdatedAt:       updatedAt,
			ShardPlacements: cloneShardPlacements(layout.ShardPlacements),
			ShardChecksums:  cloneStrings(layout.ShardChecksums),
			ShardSizes:      cloneInt64s(layout.ShardSizes),
		},
		DataChunks:   e.dataChunks,
		ParityChunks: e.parityChunks,
		TotalChunks:  e.dataChunks + e.parityChunks,
		ShardSize:    e.shardSize,
		ShardDir:     layout.ShardDir,
	}
}
