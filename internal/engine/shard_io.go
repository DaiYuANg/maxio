package engine

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/lyonbrown4d/maxio/internal/model"
)

func (e *Engine) readShardSetForRecovery(ctx context.Context, layout *Layout) ([][]byte, int, error) {
	total := e.coder.TotalChunks()
	shards := make([][]byte, total)

	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, total)
	for i := range total {
		index := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := e.readShardForRecovery(readCtx, layout, index)
			if err != nil {
				errCh <- fmt.Errorf("engine: read shard %d: %w", index, err)
				cancel()
				return
			}
			shards[index] = data
		}()
	}

	wg.Wait()
	close(errCh)
	if err := firstConcurrentError(errCh); err != nil {
		return nil, 0, err
	}

	available := 0
	for _, shard := range shards {
		if shard != nil {
			available++
		}
	}
	return shards, available, nil
}

func (e *Engine) writeShardSet(
	ctx context.Context,
	placements []model.ShardPlacement,
	shardDir string,
	hash string,
	shards [][]byte,
) error {
	indexes := make([]int, len(shards))
	for i := range shards {
		indexes[i] = i
	}
	return e.writeShardIndexes(ctx, placements, shardDir, hash, shards, indexes)
}

func (e *Engine) writeShardIndexes(
	ctx context.Context,
	placements []model.ShardPlacement,
	shardDir string,
	hash string,
	shards [][]byte,
	indexes []int,
) error {
	if err := validateShardWriteInputs(placements, shards, indexes); err != nil {
		return err
	}

	writeCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, len(indexes))
	for _, index := range indexes {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			e.writeShardIndex(writeCtx, placements, shardDir, hash, shards, index, errCh, cancel)
		}(index)
	}

	wg.Wait()
	close(errCh)
	return firstConcurrentError(errCh)
}

func validateShardWriteInputs(placements []model.ShardPlacement, shards [][]byte, indexes []int) error {
	if len(indexes) == 0 {
		return nil
	}
	if len(placements) < len(shards) {
		return errors.New("engine: shard placements do not match shards")
	}
	for _, index := range indexes {
		if index < 0 || index >= len(shards) {
			return fmt.Errorf("engine: shard index %d out of range", index)
		}
	}
	return nil
}

func (e *Engine) writeShardIndex(
	ctx context.Context,
	placements []model.ShardPlacement,
	shardDir string,
	hash string,
	shards [][]byte,
	index int,
	errCh chan<- error,
	cancel context.CancelFunc,
) {
	if err := ctx.Err(); err != nil {
		errCh <- fmt.Errorf("engine: write shard %d: %w", index, err)
		return
	}
	if err := e.writeShard(ctx, placements[index], shardDir, hash, index, shards[index]); err != nil {
		errCh <- fmt.Errorf("engine: write shard %d: %w", index, err)
		cancel()
	}
}

func firstConcurrentError(errCh <-chan error) error {
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}
