package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/spf13/afero"
)

// Default config (MinIO-style 9+3)
const (
	DefaultDataChunks   = 9
	DefaultParityChunks = 3
	DefaultShardSize    = int64(1 << 20) // 1MB shard size
)

// Engine is the erasure-coded object storage engine.
type Engine struct {
	mu           sync.RWMutex
	fs           afero.Fs
	root         string
	dataChunks   int
	parityChunks int
	shardSize    int64
	coder        *Coder
	backend      ShardStore
	layoutCache  sync.Map // string -> *Layout
	nodes        map[string]StorageNode
	drainedNodes map[string]struct{}
	controlToken string
	localNodeID  string
	planner      PlacementPlanner
}

// ObjectMeta stores object metadata (moved from metadata module).
type ObjectMeta struct {
	Bucket          string           `json:"bucket"`
	Key             string           `json:"key"`
	Hash            string           `json:"hash"`
	ETag            string           `json:"etag"`
	Size            int64            `json:"size"`
	ContentType     string           `json:"content_type"`
	UpdatedAt       time.Time        `json:"updated_at"`
	ShardPlacements []ShardPlacement `json:"shard_placements,omitempty"`
	ShardChecksums  []string         `json:"shard_checksums,omitempty"`
	ShardSizes      []int64          `json:"shard_sizes,omitempty"`
}

// ObjectInfo is ObjectMeta + erasure coding info.
type ObjectInfo struct {
	ObjectMeta
	DataChunks   int    `json:"data_chunks"`
	ParityChunks int    `json:"parity_chunks"`
	TotalChunks  int    `json:"total_chunks"`
	ShardSize    int64  `json:"shard_size"`
	ShardDir     string `json:"shard_dir"`
}

// Health reports the health of a shard set.
type Health struct {
	Bucket      string `json:"bucket"`
	Key         string `json:"key"`
	TotalChunks int    `json:"total_chunks"`
	Available   int    `json:"available"`
	Missing     int    `json:"missing"`
	Corrupted   int    `json:"corrupted"`
	Recoverable bool   `json:"recoverable"`
}

func (h Health) String() string {
	return fmt.Sprintf("health{%s:%s,%d/%d available, recoverable=%v}", h.Bucket, h.Key, h.Available, h.TotalChunks, h.Recoverable)
}

// ErrObjectNotFound is returned when an object does not exist.
var ErrObjectNotFound = errors.New("engine: object not found")

// ErrShardRecoveryFailed is returned when shard recovery fails.
var ErrShardRecoveryFailed = errors.New("engine: shard recovery failed")

// ErrShardCorrupted is returned when shard content does not match layout metadata.
var ErrShardCorrupted = errors.New("engine: shard corrupted")

// ErrObjectCorrupted is returned when decoded object content does not match its metadata hash.
var ErrObjectCorrupted = errors.New("engine: object corrupted")

func NewEngine(dataDir string, dataChunks, parityChunks int, fs afero.Fs) (*Engine, error) {
	if dataChunks < 1 {
		dataChunks = DefaultDataChunks
	}
	if parityChunks < 1 {
		parityChunks = DefaultParityChunks
	}
	if dataDir == "" {
		dataDir = "./data/engine"
	}
	if fs == nil {
		fs = afero.NewOsFs()
	}
	root := dataDir
	if !strings.HasSuffix(root, "/") {
		root += "/"
	}

	shardSize := DefaultShardSize
	backend := newShardBackend(fs, root, shardSize)
	coder, err := newCoder(CoderConfig{
		DataChunks:   dataChunks,
		ParityChunks: parityChunks,
		ShardPool:    backend,
	})
	if err != nil {
		return nil, fmt.Errorf("engine: create erasure coder: %w", err)
	}

	engine := &Engine{
		fs:           fs,
		root:         root,
		dataChunks:   dataChunks,
		parityChunks: parityChunks,
		shardSize:    shardSize,
		coder:        coder,
		backend:      backend,
	}
	engine.ConfigureLocalNode(DefaultLocalNodeID, DefaultLocalNodeAddress)
	return engine, nil
}

// PutObject writes an object to the engine with erasure coding.
func (e *Engine) PutObject(ctx context.Context, bucket, key string, reader io.Reader, contentType string) (ObjectInfo, error) {
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return ObjectInfo{}, errors.New("engine: bucket and key are required")
	}

	blob, err := e.PutBlob(ctx, key, reader)
	if err != nil {
		return ObjectInfo{}, err
	}
	return e.LinkObject(ctx, bucket, key, blob, contentType, time.Now().UTC())
}

// LinkObject persists an object layout pointing at an existing content blob.
func (e *Engine) LinkObject(
	ctx context.Context,
	bucket string,
	key string,
	blob BlobInfo,
	contentType string,
	updatedAt time.Time,
) (ObjectInfo, error) {
	_ = ctx
	bucket = strings.TrimSpace(bucket)
	key = strings.TrimSpace(key)
	if bucket == "" || key == "" {
		return ObjectInfo{}, errors.New("engine: bucket and key are required")
	}
	blob.Hash = strings.TrimSpace(blob.Hash)
	blob.ShardDir = strings.TrimSpace(blob.ShardDir)
	if blob.Hash == "" || blob.ShardDir == "" {
		return ObjectInfo{}, errors.New("engine: blob hash and shard dir are required")
	}
	if blob.ETag == "" {
		blob.ETag = ETagFromHash(blob.Hash)
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}

	layoutID := layoutHash(layoutKey(bucket, key))
	layout := &Layout{
		ID:              layoutID,
		ShardDir:        blob.ShardDir,
		Hash:            blob.Hash,
		ShardPlacements: e.resolveBlobPlacements(ctx, bucket, key, blob),
		ShardChecksums:  cloneStrings(blob.ShardChecksums),
		ShardSizes:      cloneInt64s(blob.ShardSizes),
		Bucket:          bucket,
		Key:             key,
		Size:            blob.Size,
		ETag:            blob.ETag,
		ShardSize:       e.shardSize,
		CoderType:       fmt.Sprintf("%d+%d", e.dataChunks, e.parityChunks),
		ContentType:     contentType,
		UpdatedAt:       updatedAt,
		Version:         1,
	}

	// Write metadata
	metaBytes, err := json.Marshal(layout)
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("engine: marshal layout: %w", err)
	}
	if err := e.backend.WriteMeta(blob.ShardDir, layoutID, metaBytes); err != nil {
		return ObjectInfo{}, fmt.Errorf("engine: write meta: %w", err)
	}

	// Update cache
	e.layoutCache.Store(layoutKey(bucket, key), layout)

	return ObjectInfo{
		ObjectMeta: ObjectMeta{
			Bucket:          bucket,
			Key:             key,
			Hash:            blob.Hash,
			ETag:            blob.ETag,
			Size:            blob.Size,
			ContentType:     contentType,
			UpdatedAt:       updatedAt,
			ShardPlacements: cloneShardPlacements(layout.ShardPlacements),
			ShardChecksums:  cloneStrings(layout.ShardChecksums),
			ShardSizes:      cloneInt64s(layout.ShardSizes),
		},
		DataChunks:   e.dataChunks,
		ParityChunks: e.parityChunks,
		TotalChunks:  e.dataChunks + e.parityChunks,
		ShardSize:    e.shardSize,
		ShardDir:     blob.ShardDir,
	}, nil
}

// GetObject reads and reconstructs an object from shards.
func (e *Engine) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, ObjectInfo, error) {
	_ = ctx
	layout, err := e.getLayout(bucket, key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}

	shards, available, err := e.readAvailableShards(ctx, layout)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	if ensureErr := e.ensureReadableShards(ctx, layout, shards, available); ensureErr != nil {
		return nil, ObjectInfo{}, ensureErr
	}

	// Decode data
	decoded, err := e.coder.Decode(shards, layout.Size)
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("engine: decode: %w", err)
	}
	if _, err := verifyObjectChecksum(layout, decoded); err != nil {
		return nil, ObjectInfo{}, err
	}

	return io.NopCloser(bytes.NewReader(decoded)), e.objectInfoFromLayout(layout), nil
}

// DeleteObject removes an object and its shards.
func (e *Engine) DeleteObject(ctx context.Context, bucket, key string) error {
	layout, err := e.getLayout(bucket, key)
	if err != nil {
		return err
	}

	if err := e.DeleteObjectLayout(ctx, bucket, key); err != nil {
		return err
	}
	return e.DeleteBlob(ctx, layout.ShardDir, layout.Hash)
}

// DeleteObjectLayout removes only the object-to-blob layout.
func (e *Engine) DeleteObjectLayout(ctx context.Context, bucket, key string) error {
	_ = ctx
	layout, err := e.getLayout(bucket, key)
	if err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.layoutCache.Delete(layoutKey(bucket, key))
	layoutID := layout.ID
	if layoutID == "" {
		layoutID = layoutHash(layoutKey(bucket, key))
	}
	if err := e.backend.DeleteMeta(layout.ShardDir, layoutID); err != nil {
		return fmt.Errorf("engine: delete object layout meta: %w", err)
	}
	return nil
}

// CheckHealth checks the health of a shard set.
func (e *Engine) CheckHealth(ctx context.Context, bucket, key string) (Health, error) {
	layout, err := e.getLayout(bucket, key)
	if err != nil {
		return Health{}, err
	}
	return e.healthFromLayout(ctx, layout), nil
}
