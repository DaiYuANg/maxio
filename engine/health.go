package engine

import (
	"context"
	"errors"
	"os"
)

func (e *Engine) healthFromLayout(ctx context.Context, layout *Layout) Health {
	total := e.coder.TotalChunks()
	available := 0
	missing := 0
	corrupted := 0
	for i := range total {
		data, err := e.readShard(ctx, layout, i)
		switch {
		case errors.Is(err, ErrShardCorrupted):
			corrupted++
		case errors.Is(err, os.ErrNotExist):
			missing++
		case err != nil || data == nil:
			missing++
		default:
			available++
		}
	}

	return Health{
		Bucket:      layout.Bucket,
		Key:         layout.Key,
		TotalChunks: total,
		Available:   available,
		Missing:     missing,
		Corrupted:   corrupted,
		Recoverable: available >= e.dataChunks,
	}
}
