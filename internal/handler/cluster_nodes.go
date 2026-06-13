package handler

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/control"
	"github.com/lyonbrown4d/maxio/internal/discovery"
)

const (
	ClusterNodeOnline      = "online"
	ClusterNodeSuspect     = "suspect"
	ClusterNodeOffline     = "offline"
	ClusterNodeDraining    = "draining"
	ClusterNodeDiscovered  = "discovered"
	ClusterNodeStorageOnly = "storage_only"

	ClusterStorageStateActive       = "active"
	ClusterStorageStateDrained      = "drained"
	ClusterStorageStateUnregistered = "unregistered"
)

type StorageNodeInfo struct {
	ID          string `json:"id"`
	Address     string `json:"address,omitempty"`
	Local       bool   `json:"local,omitempty"`
	Drained     bool   `json:"drained,omitempty"`
	ObjectCount int    `json:"object_count"`
	ShardCount  int    `json:"shard_count"`
	UsedBytes   int64  `json:"used_bytes"`
}

type ClusterNodeInfo struct {
	ReplicaID         uint64   `json:"replica_id,omitempty"`
	StorageNodeID     string   `json:"storage_node_id,omitempty"`
	Status            string   `json:"status"`
	Local             bool     `json:"local,omitempty"`
	Member            bool     `json:"member"`
	Removed           bool     `json:"removed,omitempty"`
	Discovered        bool     `json:"discovered"`
	StorageRegistered bool     `json:"storage_registered"`
	StorageState      string   `json:"storage_state"`
	Drained           bool     `json:"drained,omitempty"`
	ControlTarget     string   `json:"control_target,omitempty"`
	ControlAddress    string   `json:"control_address,omitempty"`
	HTTPAddress       string   `json:"http_address,omitempty"`
	StorageAddress    string   `json:"storage_address,omitempty"`
	DiscoveryState    string   `json:"discovery_state,omitempty"`
	ObjectCount       int      `json:"object_count"`
	ShardCount        int      `json:"shard_count"`
	UsedBytes         int64    `json:"used_bytes"`
	Issues            []string `json:"issues,omitempty"`
}

func (s *Service) handleClusterNodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	membership, err := s.clusterMembership(r.Context())
	if err != nil {
		s.writeError(w, err)
		return
	}
	s.writeJSON(w, http.StatusOK, BuildClusterNodeRegistry(membership, s.discoveryNodes(), s.clusterStorageNodeInfos()))
}

func BuildClusterNodeRegistry(
	membership control.Membership,
	discovered []discovery.Node,
	storageNodes []StorageNodeInfo,
) []ClusterNodeInfo {
	nodes := make(map[string]ClusterNodeInfo, len(membership.Nodes)+len(membership.Removed)+len(discovered)+len(storageNodes))
	mergeMembershipNodes(nodes, membership)
	mergeRemovedMembershipNodes(nodes, membership)
	mergeDiscoveryNodes(nodes, discovered)
	mergeStorageNodes(nodes, storageNodes)
	result := make([]ClusterNodeInfo, 0, len(nodes))
	for key := range nodes {
		node := nodes[key]
		node.StorageState = clusterStorageState(node)
		node.Status = clusterNodeStatus(node)
		node.Issues = clusterNodeIssues(node)
		result = append(result, node)
	}
	slices.SortFunc(result, compareClusterNodes)
	return result
}

func mergeMembershipNodes(nodes map[string]ClusterNodeInfo, membership control.Membership) {
	for replicaID, target := range membership.Nodes {
		key := clusterNodeKey(replicaID, clusterStorageNodeID(replicaID))
		node := nodes[key]
		node.ReplicaID = replicaID
		node.StorageNodeID = clusterStorageNodeID(replicaID)
		node.Member = true
		node.Local = replicaID == membership.LocalReplicaID
		node.ControlTarget = strings.TrimSpace(target)
		nodes[key] = node
	}
}

func mergeRemovedMembershipNodes(nodes map[string]ClusterNodeInfo, membership control.Membership) {
	for _, replicaID := range membership.Removed {
		if replicaID == 0 {
			continue
		}
		if _, active := membership.Nodes[replicaID]; active {
			continue
		}
		key := clusterNodeKey(replicaID, clusterStorageNodeID(replicaID))
		node := nodes[key]
		node.ReplicaID = replicaID
		node.StorageNodeID = clusterStorageNodeID(replicaID)
		node.Removed = true
		nodes[key] = node
	}
}

func mergeDiscoveryNodes(nodes map[string]ClusterNodeInfo, discovered []discovery.Node) {
	for index := range discovered {
		discoveredNode := discovered[index]
		if discoveredNode.ReplicaID == 0 {
			continue
		}
		key := clusterNodeKey(discoveredNode.ReplicaID, clusterStorageNodeID(discoveredNode.ReplicaID))
		node := nodes[key]
		node.ReplicaID = discoveredNode.ReplicaID
		node.StorageNodeID = clusterStorageNodeID(discoveredNode.ReplicaID)
		node.Discovered = true
		node.DiscoveryState = strings.TrimSpace(discoveredNode.State)
		node.ControlAddress = strings.TrimSpace(discoveredNode.ControlAddress)
		node.HTTPAddress = strings.TrimSpace(discoveredNode.HTTPAddress)
		nodes[key] = node
	}
}

func mergeStorageNodes(nodes map[string]ClusterNodeInfo, storageNodes []StorageNodeInfo) {
	for index := range storageNodes {
		storageNode := storageNodes[index]
		replicaID, ok := storageReplicaID(storageNode.ID)
		key := storageOnlyNodeKey(storageNode.ID)
		if ok {
			key = clusterNodeKey(replicaID, storageNode.ID)
		}
		node := nodes[key]
		if ok {
			node.ReplicaID = replicaID
		}
		node.StorageNodeID = strings.TrimSpace(storageNode.ID)
		node.StorageRegistered = true
		node.StorageAddress = strings.TrimSpace(storageNode.Address)
		node.Local = node.Local || storageNode.Local
		node.Drained = storageNode.Drained
		node.ObjectCount = storageNode.ObjectCount
		node.ShardCount = storageNode.ShardCount
		node.UsedBytes = storageNode.UsedBytes
		nodes[key] = node
	}
}

func clusterNodeStatus(node ClusterNodeInfo) string {
	if node.StorageState == ClusterStorageStateDrained {
		return ClusterNodeDraining
	}
	switch strings.ToLower(strings.TrimSpace(node.DiscoveryState)) {
	case "suspect":
		return ClusterNodeSuspect
	case "dead", "left":
		return ClusterNodeOffline
	case "alive":
		return clusterAliveStatus(node)
	default:
		return clusterUnknownDiscoveryStatus(node)
	}
}

func clusterStorageState(node ClusterNodeInfo) string {
	if !node.StorageRegistered {
		return ClusterStorageStateUnregistered
	}
	if node.Drained {
		return ClusterStorageStateDrained
	}
	return ClusterStorageStateActive
}

func clusterAliveStatus(node ClusterNodeInfo) string {
	if node.Member {
		return ClusterNodeOnline
	}
	return ClusterNodeDiscovered
}

func clusterUnknownDiscoveryStatus(node ClusterNodeInfo) string {
	switch {
	case node.Local && node.Member:
		return ClusterNodeOnline
	case node.Member:
		return ClusterNodeOffline
	case node.Removed:
		return ClusterNodeOffline
	case node.StorageRegistered:
		return ClusterNodeStorageOnly
	default:
		return ClusterNodeDiscovered
	}
}

func clusterNodeIssues(node ClusterNodeInfo) []string {
	issues := make([]string, 0, 4)
	issues = appendIssueIf(issues, node.Member && !node.Discovered && !node.Local, "not_discovered")
	issues = appendIssueIf(issues, node.Discovered && !node.Member, "not_in_control_membership")
	issues = appendIssueIf(issues, node.Removed && node.Discovered, "removed_node_reappeared")
	issues = appendIssueIf(issues, drainedStorageHasOwnership(node), "drained_storage_has_ownership")
	issues = appendIssueIf(issues, addressMismatch(node.ControlTarget, node.ControlAddress), "control_address_mismatch")
	return issues
}

func drainedStorageHasOwnership(node ClusterNodeInfo) bool {
	return node.StorageState == ClusterStorageStateDrained &&
		(node.ObjectCount > 0 || node.ShardCount > 0 || node.UsedBytes > 0)
}

func appendIssueIf(issues []string, condition bool, issue string) []string {
	if !condition {
		return issues
	}
	return append(issues, issue)
}

func addressMismatch(left, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	return left != "" && right != "" && left != right
}

func storageReplicaID(nodeID string) (uint64, bool) {
	text, ok := strings.CutPrefix(strings.TrimSpace(nodeID), "node-")
	if !ok || text == "" {
		return 0, false
	}
	replicaID, err := strconv.ParseUint(text, 10, 64)
	return replicaID, err == nil && replicaID > 0
}

func clusterNodeKey(replicaID uint64, storageNodeID string) string {
	if replicaID > 0 {
		return fmt.Sprintf("replica:%d", replicaID)
	}
	return storageOnlyNodeKey(storageNodeID)
}

func storageOnlyNodeKey(storageNodeID string) string {
	return "storage:" + strings.TrimSpace(storageNodeID)
}

func compareClusterNodes(left, right ClusterNodeInfo) int {
	if left.ReplicaID != right.ReplicaID {
		if left.ReplicaID == 0 {
			return 1
		}
		if right.ReplicaID == 0 {
			return -1
		}
		return cmp.Compare(left.ReplicaID, right.ReplicaID)
	}
	return cmp.Compare(left.StorageNodeID, right.StorageNodeID)
}
