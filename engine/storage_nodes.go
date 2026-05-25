package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func (e *Engine) RegisterStorageNode(node StorageNode) error {
	if node == nil {
		return errors.New("storage node is required")
	}
	nodeID := strings.TrimSpace(node.ID())
	if nodeID == "" {
		return errors.New("storage node id is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registerStorageNodeLocked(node)
	return nil
}

// SyncStorageNodesFromRaft replaces registered storage nodes with raft membership mapping.
func (e *Engine) SyncStorageNodesFromRaft(localReplicaID uint64, raftNodes map[uint64]string) error {
	if e == nil {
		return errors.New("storage engine is required")
	}
	if localReplicaID == 0 {
		return errors.New("local raft replica id must be greater than zero")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	drainedNodes := e.drainedNodeIDsLocked()
	localNodeID := raftStorageNodeID(localReplicaID)
	localNodeAddress := e.localNodeAddress(localNodeID)
	nextNodes, nextLocalNodeAddress, err := syncRaftStorageNodes(localReplicaID, raftNodes, e.controlToken)
	if err != nil {
		return err
	}
	if nextLocalNodeAddress != "" {
		localNodeAddress = nextLocalNodeAddress
	}

	e.nodes = map[string]StorageNode{}
	e.drainedNodes = map[string]struct{}{}
	e.configureLocalNodeLocked(localNodeID, localNodeAddress)
	for _, node := range nextNodes {
		e.nodes[node.ID()] = node
	}
	e.restoreDrainedNodesLocked(drainedNodes)
	e.reconfigurePlacementPlannerLocked()
	return nil
}

func (e *Engine) localNodeAddress(localNodeID string) string {
	address := DefaultLocalNodeAddress
	if current := e.nodes[localNodeID]; current != nil {
		address = strings.TrimSpace(current.Address())
	}
	if address == "" {
		address = DefaultLocalNodeAddress
	}
	return address
}

func (e *Engine) StorageNode(id string) (StorageNode, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	nodeID := strings.TrimSpace(id)
	if nodeID == "" {
		nodeID = e.localNodeID
	}
	if nodeID == "" {
		nodeID = DefaultLocalNodeID
	}
	node := e.nodes[nodeID]
	if node == nil {
		return nil, fmt.Errorf("storage node %q is not registered", nodeID)
	}
	return node, nil
}

func (e *Engine) LocalStorageNode() (StorageNode, error) {
	return e.StorageNode(e.localNodeID)
}

func (e *Engine) WriteLocalShard(ctx context.Context, shardDir, hash string, index int, data []byte) error {
	node, err := e.LocalStorageNode()
	if err != nil {
		return err
	}
	if err := node.WriteShard(ctx, shardDir, hash, index, data); err != nil {
		return fmt.Errorf("write local shard on node %q: %w", node.ID(), err)
	}
	return nil
}

func (e *Engine) ReadLocalShard(ctx context.Context, shardDir, hash string, index int) ([]byte, error) {
	node, err := e.LocalStorageNode()
	if err != nil {
		return nil, err
	}
	data, err := node.ReadShard(ctx, shardDir, hash, index)
	if err != nil {
		return nil, fmt.Errorf("read local shard from node %q: %w", node.ID(), err)
	}
	return data, nil
}

func (e *Engine) LocalShardExists(ctx context.Context, shardDir, hash string, index int) bool {
	node, err := e.LocalStorageNode()
	if err != nil {
		return false
	}
	return node.ShardExists(ctx, shardDir, hash, index)
}

func (e *Engine) DeleteLocalShard(ctx context.Context, shardDir, hash string, index int) error {
	node, err := e.LocalStorageNode()
	if err != nil {
		return err
	}
	if err := node.DeleteShard(ctx, shardDir, hash, index); err != nil {
		return fmt.Errorf("delete local shard from node %q: %w", node.ID(), err)
	}
	return nil
}

func syncRaftStorageNodes(
	localReplicaID uint64,
	raftNodes map[uint64]string,
	controlToken string,
) (map[string]StorageNode, string, error) {
	nodes := make(map[string]StorageNode, len(raftNodes))
	localNodeAddress := ""
	for replicaID, target := range raftNodes {
		target = strings.TrimSpace(target)
		if replicaID == 0 {
			return nil, "", errors.New("raft replica id must be greater than zero")
		}
		if target == "" {
			return nil, "", fmt.Errorf("raft target is required for replica %d", replicaID)
		}

		remote, err := newRemoteStorageNode(raftStorageNodeID(replicaID), target, nil)
		if err != nil {
			return nil, "", fmt.Errorf("build remote storage node for replica %d: %w", replicaID, err)
		}
		remote.controlToken = strings.TrimSpace(controlToken)
		nodes[raftStorageNodeID(replicaID)] = remote
		if replicaID == localReplicaID {
			localNodeAddress = strings.TrimSpace(target)
			delete(nodes, raftStorageNodeID(replicaID))
		}
	}
	return nodes, localNodeAddress, nil
}

func (e *Engine) UnregisterStorageNode(nodeID string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return errors.New("storage node id is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if nodeID == e.localNodeID {
		return errors.New("cannot unregister local storage node")
	}
	if _, ok := e.nodes[nodeID]; !ok {
		return errors.New("storage node does not exist")
	}
	delete(e.nodes, nodeID)
	delete(e.drainedNodes, nodeID)
	e.reconfigurePlacementPlannerLocked()
	return nil
}

func (e *Engine) DrainStorageNode(nodeID string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return errors.New("storage node id is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.nodes[nodeID]; !ok {
		return errors.New("storage node does not exist")
	}
	if e.drainedNodes == nil {
		e.drainedNodes = map[string]struct{}{}
	}
	e.drainedNodes[nodeID] = struct{}{}
	e.reconfigurePlacementPlannerLocked()
	return nil
}

func (e *Engine) ResumeStorageNode(nodeID string) error {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return errors.New("storage node id is required")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.nodes[nodeID]; !ok {
		return errors.New("storage node does not exist")
	}
	delete(e.drainedNodes, nodeID)
	e.reconfigurePlacementPlannerLocked()
	return nil
}

func (e *Engine) StorageNodes() []StorageNode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.storageNodesLocked()
}

func (e *Engine) registerStorageNodeLocked(node StorageNode) {
	nodeID := strings.TrimSpace(node.ID())
	if e.nodes == nil {
		e.nodes = map[string]StorageNode{}
	}
	e.nodes[nodeID] = node
	delete(e.drainedNodes, nodeID)
	e.reconfigurePlacementPlannerLocked()
}

func (e *Engine) reconfigurePlacementPlannerLocked() {
	nodes := e.placementNodesLocked()
	switch len(nodes) {
	case 0:
		e.planner = nil
	case 1:
		e.planner = NewSingleNodePlacementPlanner(nodes[0])
	default:
		e.planner = NewRoundRobinPlacementPlanner(e.localNodeID, nodes...)
	}
}

func (e *Engine) placementNodesLocked() []StorageNode {
	nodes := e.storageNodesLocked()
	if len(e.drainedNodes) == 0 {
		return nodes
	}
	active := make([]StorageNode, 0, len(nodes))
	for _, node := range nodes {
		if _, drained := e.drainedNodes[node.ID()]; !drained {
			active = append(active, node)
		}
	}
	return active
}

func (e *Engine) storageNodesLocked() []StorageNode {
	nodes := make([]StorageNode, 0, len(e.nodes))
	for _, node := range e.nodes {
		nodeID := strings.TrimSpace(node.ID())
		if nodeID == "" {
			continue
		}
		nodes = append(nodes, node)
	}
	sort.SliceStable(nodes, func(left, right int) bool {
		return strings.TrimSpace(nodes[left].ID()) < strings.TrimSpace(nodes[right].ID())
	})
	return nodes
}

func (e *Engine) drainedNodeIDsLocked() []string {
	nodeIDs := make([]string, 0, len(e.drainedNodes))
	for nodeID := range e.drainedNodes {
		nodeIDs = append(nodeIDs, nodeID)
	}
	return nodeIDs
}

func (e *Engine) restoreDrainedNodesLocked(nodeIDs []string) {
	if len(nodeIDs) == 0 {
		return
	}
	if e.drainedNodes == nil {
		e.drainedNodes = map[string]struct{}{}
	}
	for _, nodeID := range nodeIDs {
		if _, exists := e.nodes[nodeID]; exists {
			e.drainedNodes[nodeID] = struct{}{}
		}
	}
}

func cloneStorageNodes(input []StorageNode) []StorageNode {
	output := make([]StorageNode, len(input))
	copy(output, input)
	return output
}
