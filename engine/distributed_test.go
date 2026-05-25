package engine_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/config"
	"github.com/lyonbrown4d/maxio/internal/handler"
	"github.com/lyonbrown4d/maxio/model"
)

type inMemoryStorageNode struct {
	id      string
	address string
	mu      sync.RWMutex
	shards  map[string][]byte
}

func newInMemoryStorageNode(id, address string) *inMemoryStorageNode {
	return &inMemoryStorageNode{
		id:      id,
		address: address,
		shards:  make(map[string][]byte),
	}
}

func (node *inMemoryStorageNode) ID() string {
	return node.id
}

func (node *inMemoryStorageNode) Address() string {
	return node.address
}

func (node *inMemoryStorageNode) ShardKey(shardDir, hash string, index int) string {
	return fmt.Sprintf("%s|%s|%d", shardDir, hash, index)
}

func (node *inMemoryStorageNode) WriteShard(_ context.Context, shardDir, hash string, index int, data []byte) error {
	if node == nil {
		return errors.New("storage node is required")
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	node.shards[node.ShardKey(shardDir, hash, index)] = append([]byte(nil), data...)
	return nil
}

func (node *inMemoryStorageNode) ReadShard(_ context.Context, shardDir, hash string, index int) ([]byte, error) {
	if node == nil {
		return nil, errors.New("storage node is required")
	}
	node.mu.RLock()
	defer node.mu.RUnlock()
	data, ok := node.shards[node.ShardKey(shardDir, hash, index)]
	if !ok {
		return nil, fmt.Errorf("shard missing: %q", node.ShardKey(shardDir, hash, index))
	}
	return append([]byte(nil), data...), nil
}

func (node *inMemoryStorageNode) ShardExists(_ context.Context, shardDir, hash string, index int) bool {
	if node == nil {
		return false
	}
	node.mu.RLock()
	defer node.mu.RUnlock()
	_, ok := node.shards[node.ShardKey(shardDir, hash, index)]
	return ok
}

func (node *inMemoryStorageNode) DeleteShard(_ context.Context, shardDir, hash string, index int) error {
	if node == nil {
		return errors.New("storage node is required")
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	delete(node.shards, node.ShardKey(shardDir, hash, index))
	return nil
}

func (node *inMemoryStorageNode) ShardCount() int {
	if node == nil {
		return 0
	}
	node.mu.RLock()
	defer node.mu.RUnlock()
	return len(node.shards)
}

func TestPutAndGetObjectWithDistributedPlacement(t *testing.T) {
	ctx := context.Background()
	eng := newTestEngine(t)

	nodeA := newInMemoryStorageNode("node-a", "127.0.0.1:7001")
	nodeB := newInMemoryStorageNode("node-b", "127.0.0.1:7002")
	if err := registerDistributedPlacementNodes(t, eng, nodeA, nodeB); err != nil {
		t.Fatal(err)
	}

	content := []byte("distributed write path test payload")
	meta := storeObjectWithDistribution(ctx, t, eng, content)
	assertPlacementDistributed(t, meta.ShardPlacements, nodeA.id, nodeB.id)

	reader := readDistributedObject(ctx, t, eng, "test-bucket", "placement-key.txt")
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close reader: %v", closeErr)
		}
	}()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read object data: %v", err)
	}
	if !bytes.Equal(data, content) {
		t.Fatalf("data = %q, want %q", data, content)
	}
	assertRemoteNodesStoredShards(t, nodeA, nodeB)
}

func TestPutAndGetObjectWithRemoteShardHTTPTransport(t *testing.T) {
	ctx := context.Background()
	node1 := newTestEngine(t)
	node2 := newTestEngine(t)

	server := httptest.NewServer(storageShardHandler(node2))
	defer server.Close()

	if err := node1.SyncStorageNodesFromRaft(1, map[uint64]string{
		1: "127.0.0.1:9001",
		2: server.URL,
	}); err != nil {
		t.Fatalf("sync storage nodes: %v", err)
	}

	content := []byte("remote shard http transport object payload")
	meta, err := node1.PutObject(ctx, "test-bucket", "remote-http-key.txt", bytes.NewReader(content), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	assertRemoteHTTPShardsStored(ctx, t, node2, meta)

	reader, _, err := node1.GetObject(ctx, "test-bucket", "remote-http-key.txt")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			t.Fatalf("close reader: %v", closeErr)
		}
	}()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read object data: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("data = %q, want %q", got, content)
	}
}

func registerDistributedPlacementNodes(t *testing.T, e *engine.Engine, nodes ...*inMemoryStorageNode) error {
	t.Helper()
	for _, node := range nodes {
		if err := e.RegisterStorageNode(node); err != nil {
			return fmt.Errorf("register node %q: %w", node.id, err)
		}
	}
	return nil
}

func storeObjectWithDistribution(ctx context.Context, t *testing.T, e *engine.Engine, content []byte) engine.ObjectInfo {
	t.Helper()
	meta, err := e.PutObject(ctx, "test-bucket", "placement-key.txt", strings.NewReader(string(content)), "text/plain")
	if err != nil {
		t.Fatalf("PutObject: %v", err)
	}
	return meta
}

func assertPlacementDistributed(t *testing.T, placements []model.ShardPlacement, nodeA, nodeB string) {
	t.Helper()
	counts := map[string]int{}
	for _, placement := range placements {
		counts[placement.NodeID]++
	}
	if counts[engine.DefaultLocalNodeID] == 0 {
		t.Fatalf("expected shards on %q node", engine.DefaultLocalNodeID)
	}
	if counts[nodeA] == 0 {
		t.Fatalf("expected shards on %q node", nodeA)
	}
	if counts[nodeB] == 0 {
		t.Fatalf("expected shards on %q node", nodeB)
	}
}

func readDistributedObject(ctx context.Context, t *testing.T, e *engine.Engine, bucket, key string) io.ReadCloser {
	t.Helper()
	reader, _, err := e.GetObject(ctx, bucket, key)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	return reader
}

func assertRemoteNodesStoredShards(t *testing.T, nodes ...*inMemoryStorageNode) {
	t.Helper()
	for _, node := range nodes {
		if node == nil {
			t.Fatalf("node is nil")
		}
		if count := node.ShardCount(); count == 0 {
			t.Fatalf("expected shards on in-memory node %q", node.id)
		}
	}
}

func storageShardHandler(e *engine.Engine) *http.ServeMux {
	logger := slog.New(slog.DiscardHandler)
	service := handler.NewService(handler.NewDependencies(nil, e, nil, nil, nil, nil), logger, config.Config{})
	router := http.NewServeMux()
	service.RegisterHTTP(router)
	return router
}

func assertRemoteHTTPShardsStored(ctx context.Context, t *testing.T, e *engine.Engine, meta engine.ObjectInfo) {
	t.Helper()
	remoteShards := 0
	for _, placement := range meta.ShardPlacements {
		if placement.NodeID != "raft-2" {
			continue
		}
		remoteShards++
		if !e.LocalShardExists(ctx, meta.ShardDir, meta.Hash, placement.Index) {
			t.Fatalf("expected shard %d on remote HTTP node", placement.Index)
		}
	}
	if remoteShards == 0 {
		t.Fatal("expected at least one shard placed on remote HTTP node")
	}
}
