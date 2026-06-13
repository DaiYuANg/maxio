package handler

import (
	"cmp"
	"slices"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/discovery"
)

func mergeDiscoveredMembers(
	desired map[uint64]string,
	current map[uint64]string,
	removed map[uint64]struct{},
	nodes []discovery.Node,
) []clusterDiscoveryConflict {
	conflicts := make([]clusterDiscoveryConflict, 0)
	for index := range nodes {
		conflict, ok := mergeDiscoveredMember(desired, current, removed, nodes[index])
		if ok {
			conflicts = append(conflicts, conflict)
		}
	}
	slices.SortFunc(conflicts, compareClusterDiscoveryConflicts)
	return conflicts
}

func mergeDiscoveredMember(
	desired map[uint64]string,
	current map[uint64]string,
	removed map[uint64]struct{},
	node discovery.Node,
) (clusterDiscoveryConflict, bool) {
	if !usableDiscoveredNode(node) {
		return clusterDiscoveryConflict{}, false
	}
	target := strings.TrimSpace(node.ControlAddress)
	if _, ok := removed[node.ReplicaID]; ok {
		return removedReplicaReappearedConflict(node.ReplicaID, target), true
	}
	if existing, ok := current[node.ReplicaID]; ok && existing != target {
		desired[node.ReplicaID] = existing
		return addressChangeConflict(node.ReplicaID, existing, target), true
	}
	if existing, ok := desired[node.ReplicaID]; ok && existing != target {
		return addressChangeConflict(node.ReplicaID, existing, target), true
	}
	desired[node.ReplicaID] = target
	return clusterDiscoveryConflict{}, false
}

func removedReplicaReappearedConflict(replicaID uint64, target string) clusterDiscoveryConflict {
	return clusterDiscoveryConflict{
		ReplicaID:  replicaID,
		Reason:     clusterMembershipReasonRemovedReplicaReappeared,
		Current:    "removed",
		Discovered: target,
	}
}

func addressChangeConflict(replicaID uint64, current, discovered string) clusterDiscoveryConflict {
	return clusterDiscoveryConflict{
		ReplicaID:  replicaID,
		Reason:     clusterMembershipReasonAddressChangeBlocked,
		Current:    current,
		Discovered: discovered,
	}
}

func compareClusterDiscoveryConflicts(left, right clusterDiscoveryConflict) int {
	if byReplica := cmp.Compare(left.ReplicaID, right.ReplicaID); byReplica != 0 {
		return byReplica
	}
	return cmp.Compare(left.Reason, right.Reason)
}
