package engine

import (
	"context"
	"fmt"
	"strings"
)

// ScrubResult reports object and shard verification results for one object.
type ScrubResult struct {
	Health         Health `json:"health"`
	Healthy        bool   `json:"healthy"`
	ObjectVerified bool   `json:"object_verified"`
	ExpectedHash   string `json:"expected_hash,omitempty"`
	ActualHash     string `json:"actual_hash,omitempty"`
}

// ScrubObject verifies shard checksums and the decoded object checksum.
func (e *Engine) ScrubObject(ctx context.Context, bucket, key string) (ScrubResult, error) {
	layout, err := e.getLayout(bucket, key)
	if err != nil {
		return ScrubResult{}, err
	}

	result := ScrubResult{
		Health:       e.healthFromLayout(ctx, layout),
		ExpectedHash: strings.TrimSpace(layout.Hash),
	}
	if result.Health.Missing != 0 || result.Health.Corrupted != 0 {
		return result, nil
	}

	shards, available, err := e.readAvailableShards(ctx, layout)
	if err != nil {
		return result, fmt.Errorf("engine: read scrub shards: %w", err)
	}
	if readableErr := e.ensureReadableShards(ctx, layout, shards, available); readableErr != nil {
		return result, fmt.Errorf("engine: prepare scrub shards: %w", readableErr)
	}

	decoded, err := e.coder.Decode(shards, layout.Size)
	if err != nil {
		return result, fmt.Errorf("engine: decode scrub object: %w", err)
	}

	actual, err := verifyObjectChecksum(layout, decoded)
	result.ActualHash = actual
	result.ObjectVerified = true
	result.Healthy = err == nil
	if err != nil {
		return result, err
	}
	return result, nil
}

func verifyObjectChecksum(layout *Layout, data []byte) (string, error) {
	if layout == nil {
		return "", nil
	}
	expected := strings.TrimSpace(layout.Hash)
	if expected == "" {
		return "", nil
	}
	actual := HashBytes(data)
	if actual != expected {
		return actual, fmt.Errorf("%w: expected=%s actual=%s", ErrObjectCorrupted, expected, actual)
	}
	return actual, nil
}
