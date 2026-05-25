package store_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/lyonbrown4d/maxio/engine"
	"github.com/lyonbrown4d/maxio/internal/metadata"
	"github.com/lyonbrown4d/maxio/internal/store"
	"github.com/lyonbrown4d/maxio/model"
	"github.com/spf13/afero"
)

type memoryStorageNode struct {
	id     string
	mu     sync.RWMutex
	shards map[string][]byte
}

func newMemoryStorageNode(id string) *memoryStorageNode {
	return &memoryStorageNode{id: id, shards: map[string][]byte{}}
}

func (node *memoryStorageNode) ID() string {
	return node.id
}

func (node *memoryStorageNode) Address() string {
	return node.id
}

func (node *memoryStorageNode) WriteShard(_ context.Context, shardDir, hash string, index int, data []byte) error {
	node.mu.Lock()
	defer node.mu.Unlock()
	node.shards[node.shardKey(shardDir, hash, index)] = append([]byte(nil), data...)
	return nil
}

func (node *memoryStorageNode) ReadShard(_ context.Context, shardDir, hash string, index int) ([]byte, error) {
	node.mu.RLock()
	defer node.mu.RUnlock()
	data, ok := node.shards[node.shardKey(shardDir, hash, index)]
	if !ok {
		return nil, errors.New("shard missing")
	}
	return append([]byte(nil), data...), nil
}

func (node *memoryStorageNode) ShardExists(_ context.Context, shardDir, hash string, index int) bool {
	node.mu.RLock()
	defer node.mu.RUnlock()
	_, ok := node.shards[node.shardKey(shardDir, hash, index)]
	return ok
}

func (node *memoryStorageNode) DeleteShard(_ context.Context, shardDir, hash string, index int) error {
	node.mu.Lock()
	defer node.mu.Unlock()
	delete(node.shards, node.shardKey(shardDir, hash, index))
	return nil
}

func (node *memoryStorageNode) shardKey(shardDir, hash string, index int) string {
	return fmt.Sprintf("%s/%s/%d", shardDir, hash, index)
}

func TestStoreRebalanceNodeUpdatesObjectAndBlobRefPlacements(t *testing.T) {
	ctx := context.Background()
	meta := metadata.NewInMemoryMetadata()
	eng, err := engine.NewEngine("/test", engine.DefaultDataChunks, engine.DefaultParityChunks, afero.NewMemMapFs())
	mustNoError(t, err, "new engine")
	nodeA := newMemoryStorageNode("node-a")
	nodeB := newMemoryStorageNode("node-b")
	mustNoError(t, eng.RegisterStorageNode(nodeA), "register node a")
	mustNoError(t, eng.RegisterStorageNode(nodeB), "register node b")
	storeModule, err := store.NewStore("", meta, eng)
	mustNoError(t, err, "new store")
	mustNoError(t, storeModule.CreateBucket(ctx, "bucket"), "create bucket")

	objectMeta, err := storeModule.PutObject(ctx, "bucket", "object.txt", strings.NewReader("rebalance payload"), store.PutOptions{
		ContentType: "text/plain",
	})
	mustNoError(t, err, "put object")
	assertPlacementsUseNode(t, objectMeta.ShardPlacements, nodeA.id)
	mustNoError(t, eng.DrainStorageNode(nodeA.id), "drain node a")

	result, err := storeModule.RebalanceNode(ctx, nodeA.id)
	mustNoError(t, err, "rebalance node")
	mustEqual(t, result.NodeID, nodeA.id, "result node id")
	if result.Shards == 0 {
		t.Fatal("expected moved shards")
	}

	updatedObjects, err := storeModule.ListObjects(ctx, "bucket", "")
	mustNoError(t, err, "list objects")
	assertPlacementsExcludeNode(t, updatedObjects[0].ShardPlacements, nodeA.id)
	ref, ok, err := meta.GetBlobRef(ctx, objectMeta.Hash)
	mustNoError(t, err, "get blob ref")
	if !ok {
		t.Fatal("blob ref not found")
	}
	assertPlacementsExcludeNode(t, ref.ShardPlacements, nodeA.id)
}

func assertPlacementsUseNode(t *testing.T, placements []model.ShardPlacement, nodeID string) {
	t.Helper()
	for _, placement := range placements {
		if placement.NodeID == nodeID {
			return
		}
	}
	t.Fatalf("placements do not use node %q", nodeID)
}

func assertPlacementsExcludeNode(t *testing.T, placements []model.ShardPlacement, nodeID string) {
	t.Helper()
	for _, placement := range placements {
		if placement.NodeID == nodeID {
			t.Fatalf("placements still use node %q", nodeID)
		}
	}
}
