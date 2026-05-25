package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/model"
)

const (
	DefaultLocalNodeID      = "local"
	DefaultLocalNodeAddress = "local"
)

type PlacementRequest struct {
	Bucket     string
	Key        string
	Hash       string
	ShardCount int
}

type PlacementPlan struct {
	Shards []model.ShardPlacement
}

type PlacementPlanner interface {
	Plan(ctx context.Context, request PlacementRequest) (PlacementPlan, error)
}

type SingleNodePlacementPlanner struct {
	node StorageNode
}

func NewSingleNodePlacementPlanner(node StorageNode) *SingleNodePlacementPlanner {
	return &SingleNodePlacementPlanner{node: node}
}

func (planner *SingleNodePlacementPlanner) Plan(ctx context.Context, request PlacementRequest) (PlacementPlan, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return PlacementPlan{}, fmt.Errorf("plan shard placement context: %w", err)
		}
	}
	if planner == nil || planner.node == nil {
		return PlacementPlan{}, errors.New("placement planner node is required")
	}
	if request.ShardCount < 1 {
		return PlacementPlan{}, errors.New("placement shard count must be greater than zero")
	}
	placements := make([]model.ShardPlacement, request.ShardCount)
	for index := range placements {
		placements[index] = model.ShardPlacement{
			Index:       index,
			NodeID:      planner.node.ID(),
			NodeAddress: planner.node.Address(),
			Local:       true,
		}
	}
	return PlacementPlan{Shards: placements}, nil
}

type RoundRobinPlacementPlanner struct {
	localNodeID string
	nodes       []StorageNode
}

func NewRoundRobinPlacementPlanner(localNodeID string, nodes ...StorageNode) *RoundRobinPlacementPlanner {
	normalized := make([]StorageNode, 0, len(nodes))
	for _, node := range nodes {
		if node != nil && strings.TrimSpace(node.ID()) != "" {
			normalized = append(normalized, node)
		}
	}
	sort.SliceStable(normalized, func(left, right int) bool {
		return normalized[left].ID() < normalized[right].ID()
	})
	return &RoundRobinPlacementPlanner{
		localNodeID: strings.TrimSpace(localNodeID),
		nodes:       cloneStorageNodes(normalized),
	}
}

func (planner *RoundRobinPlacementPlanner) Plan(ctx context.Context, request PlacementRequest) (PlacementPlan, error) {
	if planner == nil {
		return PlacementPlan{}, errors.New("placement planner is required")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return PlacementPlan{}, fmt.Errorf("plan shard placement context: %w", err)
		}
	}
	if request.ShardCount < 1 {
		return PlacementPlan{}, errors.New("placement shard count must be greater than zero")
	}
	if len(planner.nodes) == 0 {
		return PlacementPlan{}, errors.New("placement planner needs at least one storage node")
	}
	placements := make([]model.ShardPlacement, request.ShardCount)
	for index := range placements {
		node := planner.nodes[index%len(planner.nodes)]
		nodeID := strings.TrimSpace(node.ID())
		placements[index] = model.ShardPlacement{
			Index:       index,
			NodeID:      nodeID,
			NodeAddress: strings.TrimSpace(node.Address()),
			Local:       nodeID != "" && nodeID == planner.localNodeID,
		}
	}
	return PlacementPlan{Shards: placements}, nil
}

func (e *Engine) ConfigureLocalNode(id, address string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.configureLocalNodeLocked(id, address)
}

func (e *Engine) configureLocalNodeLocked(id, address string) {
	id = strings.TrimSpace(id)
	address = strings.TrimSpace(address)
	if id == "" {
		id = DefaultLocalNodeID
	}
	if address == "" {
		address = DefaultLocalNodeAddress
	}
	node := NewLocalStorageNode(id, address, e.backend)
	e.localNodeID = node.ID()
	e.nodes = map[string]StorageNode{}
	e.drainedNodes = map[string]struct{}{}
	e.registerStorageNodeLocked(node)
}

func (e *Engine) PlanShardPlacement(ctx context.Context, request PlacementRequest) ([]model.ShardPlacement, error) {
	e.mu.RLock()
	planner := e.planner
	e.mu.RUnlock()
	if planner == nil {
		return nil, errors.New("placement planner is not configured")
	}
	plan, err := planner.Plan(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("plan shard placement: %w", err)
	}
	if err := validatePlacementPlan(plan, request.ShardCount); err != nil {
		return nil, err
	}
	return cloneShardPlacements(plan.Shards), nil
}

func validatePlacementPlan(plan PlacementPlan, shardCount int) error {
	if len(plan.Shards) != shardCount {
		return fmt.Errorf("placement plan shard count = %d, want %d", len(plan.Shards), shardCount)
	}
	for index := range plan.Shards {
		placement := plan.Shards[index]
		if placement.Index != index {
			return fmt.Errorf("placement index = %d, want %d", placement.Index, index)
		}
		if strings.TrimSpace(placement.NodeID) == "" {
			return fmt.Errorf("placement node id is required for shard %d", index)
		}
	}
	return nil
}

func (e *Engine) resolveBlobPlacements(ctx context.Context, bucket, key string, blob BlobInfo) []model.ShardPlacement {
	if len(blob.ShardPlacements) == e.coder.TotalChunks() {
		return cloneShardPlacements(blob.ShardPlacements)
	}
	placements, err := e.PlanShardPlacement(ctx, PlacementRequest{
		Bucket:     bucket,
		Key:        key,
		Hash:       blob.Hash,
		ShardCount: e.coder.TotalChunks(),
	})
	if err != nil {
		return e.localShardPlacements()
	}
	return placements
}

func (e *Engine) localShardPlacements() []model.ShardPlacement {
	total := e.coder.TotalChunks()
	placements := make([]model.ShardPlacement, total)
	for index := range placements {
		placements[index] = e.localShardPlacement(index)
	}
	return placements
}

func (e *Engine) localShardPlacement(index int) model.ShardPlacement {
	e.mu.RLock()
	node := e.nodes[e.localNodeID]
	e.mu.RUnlock()
	if node == nil {
		return model.ShardPlacement{
			Index:       index,
			NodeID:      DefaultLocalNodeID,
			NodeAddress: DefaultLocalNodeAddress,
			Local:       true,
		}
	}
	return model.ShardPlacement{
		Index:       index,
		NodeID:      node.ID(),
		NodeAddress: node.Address(),
		Local:       true,
	}
}

func (e *Engine) shardPlacement(layout *Layout, index int) model.ShardPlacement {
	if layout != nil && index >= 0 && index < len(layout.ShardPlacements) {
		placement := layout.ShardPlacements[index]
		if strings.TrimSpace(placement.NodeID) != "" {
			return placement
		}
	}
	return e.localShardPlacement(index)
}

func (e *Engine) shardPlacements(layout *Layout) []model.ShardPlacement {
	total := e.coder.TotalChunks()
	placements := make([]model.ShardPlacement, total)
	for index := range placements {
		placements[index] = e.shardPlacement(layout, index)
	}
	return placements
}

func cloneShardPlacements(input []model.ShardPlacement) []model.ShardPlacement {
	if len(input) == 0 {
		return nil
	}
	output := make([]model.ShardPlacement, len(input))
	copy(output, input)
	return output
}
