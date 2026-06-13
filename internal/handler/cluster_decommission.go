package handler

import "context"

func (s *Service) ensureClusterMemberDecommissionable(ctx context.Context, replicaID uint64) error {
	_, _, _ = s, ctx, replicaID
	return nil
}

type clusterDecommissionBlockedError struct {
	replicaID uint64
	nodeID    string
	stats     nodePlacementStats
}

func (e *clusterDecommissionBlockedError) Error() string {
	return "cluster member decommission blocked by local data placement"
}

func (e *clusterDecommissionBlockedError) Unwrap() error {
	return errClusterDecommissionBlocked
}
