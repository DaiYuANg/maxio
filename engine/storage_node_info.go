package engine

import "strings"

type StorageNodeInfo struct {
	ID          string `json:"id"`
	Address     string `json:"address"`
	Local       bool   `json:"local"`
	Drained     bool   `json:"drained"`
	ObjectCount int    `json:"object_count"`
	ShardCount  int    `json:"shard_count"`
	UsedBytes   int64  `json:"used_bytes"`
}

type storageNodeOwnership struct {
	objects int
	shards  int
	bytes   int64
}

func (e *Engine) StorageNodeInfos() []StorageNodeInfo {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()

	nodes := e.storageNodesLocked()
	ownership := e.storageNodeOwnershipLocked()
	infos := make([]StorageNodeInfo, 0, len(nodes))
	for _, node := range nodes {
		nodeID := node.ID()
		_, drained := e.drainedNodes[nodeID]
		nodeOwnership := ownership[nodeID]
		infos = append(infos, StorageNodeInfo{
			ID:          nodeID,
			Address:     node.Address(),
			Local:       nodeID == e.localNodeID,
			Drained:     drained,
			ObjectCount: nodeOwnership.objects,
			ShardCount:  nodeOwnership.shards,
			UsedBytes:   nodeOwnership.bytes,
		})
	}
	return infos
}

func (e *Engine) storageNodeOwnershipLocked() map[string]storageNodeOwnership {
	ownership := make(map[string]storageNodeOwnership)
	localNodeID := e.localNodeID
	if localNodeID == "" {
		localNodeID = DefaultLocalNodeID
	}
	e.layoutCache.Range(func(_, value any) bool {
		layout, ok := value.(*Layout)
		if !ok || layout == nil {
			return true
		}
		countLayoutOwnership(ownership, localNodeID, layout.ShardPlacements, layout.ShardSizes)
		return true
	})
	return ownership
}

func countLayoutOwnership(
	ownership map[string]storageNodeOwnership,
	localNodeID string,
	placements []ShardPlacement,
	sizes []int64,
) {
	seenObjects := make(map[string]struct{})
	for index := range placements {
		nodeID := strings.TrimSpace(placements[index].NodeID)
		if nodeID == "" {
			nodeID = localNodeID
		}
		nodeOwnership := ownership[nodeID]
		nodeOwnership.shards++
		nodeOwnership.bytes += shardSizeAt(sizes, placements[index].Index)
		ownership[nodeID] = nodeOwnership
		seenObjects[nodeID] = struct{}{}
	}
	for nodeID := range seenObjects {
		nodeOwnership := ownership[nodeID]
		nodeOwnership.objects++
		ownership[nodeID] = nodeOwnership
	}
}

func shardSizeAt(sizes []int64, index int) int64 {
	if index < 0 || index >= len(sizes) {
		return 0
	}
	return sizes[index]
}
