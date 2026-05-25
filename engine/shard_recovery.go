package engine

import (
	"context"
	"errors"
	"os"
)

func (e *Engine) readShardForRecovery(ctx context.Context, layout *Layout, index int) ([]byte, error) {
	data, err := e.readShard(ctx, layout, index)
	if isUnavailableShardError(err) {
		return nil, nil
	}
	return data, err
}

func (e *Engine) countReadableShards(ctx context.Context, layout *Layout) int {
	total := e.coder.TotalChunks()
	available := 0
	for i := range total {
		data, err := e.readShardForRecovery(ctx, layout, i)
		if err == nil && data != nil {
			available++
		}
	}
	return available
}

func isUnavailableShardError(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrShardCorrupted)
}
