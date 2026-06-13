package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type StorageNode interface {
	ID() string
	Address() string
	WriteShard(ctx context.Context, shardDir, hash string, index int, data []byte) error
	ReadShard(ctx context.Context, shardDir, hash string, index int) ([]byte, error)
	ShardExists(ctx context.Context, shardDir, hash string, index int) bool
	DeleteShard(ctx context.Context, shardDir, hash string, index int) error
}

type LocalStorageNode struct {
	id      string
	address string
	store   ShardStore
}

func NewLocalStorageNode(id, address string, store ShardStore) *LocalStorageNode {
	id = strings.TrimSpace(id)
	address = strings.TrimSpace(address)
	if id == "" {
		id = DefaultLocalNodeID
	}
	if address == "" {
		address = DefaultLocalNodeAddress
	}
	return &LocalStorageNode{
		id:      id,
		address: address,
		store:   store,
	}
}

func (node *LocalStorageNode) ID() string {
	if node == nil || node.id == "" {
		return DefaultLocalNodeID
	}
	return node.id
}

func (node *LocalStorageNode) Address() string {
	if node == nil || node.address == "" {
		return DefaultLocalNodeAddress
	}
	return node.address
}

func (node *LocalStorageNode) WriteShard(ctx context.Context, shardDir, hash string, index int, data []byte) error {
	if err := contextError(ctx, "write local shard"); err != nil {
		return err
	}
	if node == nil || node.store == nil {
		return errors.New("local storage node store is required")
	}
	if err := node.store.WriteShard(shardDir, hash, index, data); err != nil {
		return fmt.Errorf("write local shard: %w", err)
	}
	return nil
}

func (node *LocalStorageNode) ReadShard(ctx context.Context, shardDir, hash string, index int) ([]byte, error) {
	if err := contextError(ctx, "read local shard"); err != nil {
		return nil, err
	}
	if node == nil || node.store == nil {
		return nil, errors.New("local storage node store is required")
	}
	data, err := node.store.ReadShard(shardDir, hash, index)
	if err != nil {
		return nil, fmt.Errorf("read local shard: %w", err)
	}
	return data, nil
}

func (node *LocalStorageNode) ShardExists(ctx context.Context, shardDir, hash string, index int) bool {
	if contextError(ctx, "check local shard") != nil {
		return false
	}
	if node == nil || node.store == nil {
		return false
	}
	return node.store.ShardExists(shardDir, hash, index)
}

func (node *LocalStorageNode) DeleteShard(ctx context.Context, shardDir, hash string, index int) error {
	if err := contextError(ctx, "delete local shard"); err != nil {
		return err
	}
	if node == nil || node.store == nil {
		return errors.New("local storage node store is required")
	}
	if err := node.store.DeleteShard(shardDir, hash, index); err != nil {
		return fmt.Errorf("delete local shard: %w", err)
	}
	return nil
}

func clusterStorageNodeID(replicaID uint64) string {
	return fmt.Sprintf("node-%d", replicaID)
}

func (e *Engine) writeShard(ctx context.Context, placement ShardPlacement, shardDir, hash string, index int, data []byte) error {
	node, err := e.storageNode(placement)
	if err != nil {
		return err
	}
	if err := node.WriteShard(ctx, shardDir, hash, index, data); err != nil {
		return fmt.Errorf("write shard to node %q: %w", node.ID(), err)
	}
	return nil
}

func (e *Engine) readShard(ctx context.Context, layout *Layout, index int) ([]byte, error) {
	placement := e.shardPlacement(layout, index)
	node, err := e.storageNode(placement)
	if err != nil {
		return nil, err
	}
	data, err := node.ReadShard(ctx, layout.ShardDir, layout.Hash, index)
	if err != nil {
		return nil, fmt.Errorf("read shard from node %q: %w", node.ID(), err)
	}
	if err := verifyShardChecksum(layout, index, data); err != nil {
		return nil, fmt.Errorf("verify shard from node %q: %w", node.ID(), err)
	}
	return data, nil
}

func verifyShardChecksum(layout *Layout, index int, data []byte) error {
	if data == nil || layout == nil || index < 0 || index >= len(layout.ShardChecksums) {
		return nil
	}
	expected := strings.TrimSpace(layout.ShardChecksums[index])
	if expected == "" {
		return nil
	}
	actual := HashBytes(data)
	if actual != expected {
		return fmt.Errorf("%w: index=%d expected=%s actual=%s", ErrShardCorrupted, index, expected, actual)
	}
	return nil
}

func (e *Engine) storageNode(placement ShardPlacement) (StorageNode, error) {
	nodeID := strings.TrimSpace(placement.NodeID)
	if nodeID == "" {
		nodeID = e.localNodeID
	}
	e.mu.RLock()
	node := e.nodes[nodeID]
	e.mu.RUnlock()
	if node == nil {
		return nil, fmt.Errorf("storage node %q is not registered", nodeID)
	}
	return node, nil
}

func contextError(ctx context.Context, operation string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s context: %w", operation, err)
	}
	return nil
}
