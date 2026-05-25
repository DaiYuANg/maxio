package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

type ShardSetRef struct {
	ShardDir string
	Hash     string
}

type OrphanShardSet struct {
	ShardDir string `json:"shard_dir"`
	Hash     string `json:"hash"`
}

type OrphanShardCleanupResult struct {
	Scanned int              `json:"scanned"`
	Removed int              `json:"removed"`
	Orphans []OrphanShardSet `json:"orphans,omitempty"`
}

func (e *Engine) CleanupOrphanShardSets(
	ctx context.Context,
	liveRefs []ShardSetRef,
	dryRun bool,
) (OrphanShardCleanupResult, error) {
	if e == nil || e.backend == nil {
		return OrphanShardCleanupResult{}, errors.New("engine is not ready")
	}
	if err := ctxErr(ctx); err != nil {
		return OrphanShardCleanupResult{}, err
	}
	local, err := e.localShardSets()
	if err != nil {
		return OrphanShardCleanupResult{}, err
	}
	live := liveShardSetMap(liveRefs)
	result := OrphanShardCleanupResult{Scanned: len(local)}
	for index := range local {
		if err := e.cleanupShardSet(ctx, local[index], live, dryRun, &result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (e *Engine) cleanupShardSet(
	ctx context.Context,
	set OrphanShardSet,
	live map[string]struct{},
	dryRun bool,
	result *OrphanShardCleanupResult,
) error {
	if _, ok := live[shardSetKey(set.ShardDir, set.Hash)]; ok {
		return nil
	}
	result.Orphans = append(result.Orphans, set)
	if dryRun {
		return nil
	}
	if err := e.DeleteBlob(ctx, set.ShardDir, set.Hash); err != nil {
		return fmt.Errorf("delete orphan shard set %s/%s: %w", set.ShardDir, set.Hash, err)
	}
	result.Removed++
	return nil
}

func (e *Engine) localShardSets() ([]OrphanShardSet, error) {
	shardDirs, err := afero.ReadDir(e.fs, e.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read shard root: %w", err)
	}
	sets := make([]OrphanShardSet, 0)
	for _, shardDir := range shardDirs {
		if !shardDir.IsDir() {
			continue
		}
		discovered, err := e.localShardSetsInDir(shardDir.Name())
		if err != nil {
			return nil, err
		}
		sets = append(sets, discovered...)
	}
	return sets, nil
}

func (e *Engine) localShardSetsInDir(shardDir string) ([]OrphanShardSet, error) {
	path := filepath.Join(e.root, shardDir)
	entries, err := afero.ReadDir(e.fs, path)
	if err != nil {
		return nil, fmt.Errorf("read shard dir %q: %w", shardDir, err)
	}
	sets := make([]OrphanShardSet, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if hasShardChunks(e.fs, filepath.Join(path, entry.Name())) {
			sets = append(sets, OrphanShardSet{ShardDir: shardDir, Hash: entry.Name()})
		}
	}
	return sets, nil
}

func hasShardChunks(fs afero.Fs, dir string) bool {
	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "chunk-") {
			return true
		}
	}
	return false
}

func liveShardSetMap(refs []ShardSetRef) map[string]struct{} {
	live := make(map[string]struct{}, len(refs))
	for index := range refs {
		ref := refs[index]
		ref.ShardDir = strings.TrimSpace(ref.ShardDir)
		ref.Hash = strings.TrimSpace(ref.Hash)
		if ref.ShardDir == "" || ref.Hash == "" {
			continue
		}
		live[shardSetKey(ref.ShardDir, ref.Hash)] = struct{}{}
	}
	return live
}

func shardSetKey(shardDir, hash string) string {
	return shardDir + "\x00" + hash
}

func ctxErr(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("engine context: %w", err)
	}
	return nil
}
