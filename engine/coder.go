package engine

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/klauspost/reedsolomon"
)

// Coder encodes and decodes objects using Reed-Solomon erasure coding.
// k = data chunks, m = parity chunks (configurable, defaults to MinIO: 9+3)
type Coder struct {
	k      int
	m      int
	pool   ShardStore
	scheme reedsolomon.Encoder
}

// CoderConfig holds parameters for constructing a Coder.
type CoderConfig struct {
	DataChunks   int
	ParityChunks int
	ShardPool    ShardStore
}

func (cfg CoderConfig) validate() error {
	if cfg.DataChunks < 1 {
		return errors.New("erasure: data chunks must be >= 1")
	}
	if cfg.ParityChunks < 1 {
		return errors.New("erasure: parity chunks must be >= 1")
	}
	if cfg.ShardPool == nil {
		return errors.New("erasure: shard pool must be non-nil")
	}
	return nil
}

func newCoder(cfg CoderConfig) (*Coder, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	scheme, err := reedsolomon.New(cfg.DataChunks, cfg.ParityChunks)
	if err != nil {
		return nil, fmt.Errorf("erasure: create encoder: %w", err)
	}
	return &Coder{
		k:      cfg.DataChunks,
		m:      cfg.ParityChunks,
		pool:   cfg.ShardPool,
		scheme: scheme,
	}, nil
}

func (c *Coder) DataChunks() int   { return c.k }
func (c *Coder) ParityChunks() int { return c.m }
func (c *Coder) TotalChunks() int  { return c.k + c.m }
func (c *Coder) Pool() ShardStore  { return c.pool }

// Encode encodes a single object, splitting it into k data shards and m parity shards.
func (c *Coder) Encode(data []byte) ([][]byte, error) {
	numChunks := c.TotalChunks()
	shardSize := (len(data) + c.k - 1) / c.k
	padded := make([][]byte, numChunks)
	for i := range numChunks {
		padded[i] = make([]byte, shardSize)
	}
	for i := range c.k {
		start := i * shardSize
		if start >= len(data) {
			continue
		}
		end := min(start+shardSize, len(data))
		copy(padded[i], data[start:end])
	}
	// Fill remaining data shard slots with zeros
	for i := range c.k {
		start := i * shardSize
		if start >= len(data) {
			padded[i] = make([]byte, shardSize)
		}
	}
	if err := c.scheme.Encode(padded); err != nil {
		return nil, fmt.Errorf("erasure: encode data: %w", err)
	}
	return padded, nil
}

// Decode reconstructs the original data from shards. At most m shards may be missing.
func (c *Coder) Decode(shards [][]byte, originalSize int64) ([]byte, error) {
	if originalSize < 0 {
		return nil, errors.New("erasure: original size must be >= 0")
	}
	if originalSize == 0 {
		return []byte{}, nil
	}
	shardSize := availableShardSize(shards)
	if shardSize == 0 {
		return nil, errors.New("erasure: no shards available")
	}
	if err := c.scheme.Reconstruct(shards); err != nil {
		return nil, fmt.Errorf("erasure: decode data: %w", err)
	}
	if err := c.ensureDataShardsReady(shards); err != nil {
		return nil, err
	}
	// Reassemble k data shards and truncate to the exact object size.
	reconstructed := bytes.Join(shards[:c.k], nil)
	if originalSize > int64(len(reconstructed)) {
		return nil, errors.New("erasure: original size exceeds reconstructed payload")
	}
	return reconstructed[:int(originalSize)], nil
}

func (c *Coder) ensureDataShardsReady(shards [][]byte) error {
	for i := range c.k {
		if len(shards[i]) == 0 {
			return fmt.Errorf("erasure: data shard %d was not reconstructed", i)
		}
	}
	return nil
}

func availableShardSize(shards [][]byte) int {
	for _, shard := range shards {
		if len(shard) > 0 {
			return len(shard)
		}
	}
	return 0
}

// Rebuild regenerates missing data or parity shards from remaining shards.
func (c *Coder) Rebuild(shards [][]byte) error {
	if err := c.scheme.Reconstruct(shards); err != nil {
		return fmt.Errorf("erasure: rebuild shards: %w", err)
	}
	return nil
}
