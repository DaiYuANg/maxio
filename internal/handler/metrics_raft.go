package handler

import (
	"context"
	"errors"

	raftx "github.com/lyonbrown4d/maxio/internal/raft"
)

func (collector *metricsCollector) addRaftStatus(ctx context.Context, s *Service) {
	if s == nil || s.raft == nil {
		collector.gaugeUint64("maxio_raft_local_replica_id", "Local raft replica ID.", 0)
		collector.gauge("maxio_raft_leader_available", "Whether a raft leader is currently known.", 0)
		collector.gauge("maxio_raft_local_is_leader", "Whether the local raft replica is the current leader.", 0)
		collector.addRaftMembershipMetrics(raftx.Membership{})
		return
	}
	collector.gaugeUint64("maxio_raft_local_replica_id", "Local raft replica ID.", s.raft.LocalReplicaID())
	collector.addRaftLeaderStatus(s.raft.AssertLeader(ctx))

	membership, err := s.raft.GetMembership(ctx)
	if err != nil {
		collector.collectionErrors++
		return
	}
	collector.addRaftMembershipMetrics(membership)
}

func (collector *metricsCollector) addRaftLeaderStatus(err error) {
	collector.gauge("maxio_raft_leader_available", "Whether a raft leader is currently known.", boolInt(!errors.Is(err, raftx.ErrLeaderUnavailable)))
	collector.gauge("maxio_raft_local_is_leader", "Whether the local raft replica is the current leader.", boolInt(err == nil))
}

func (collector *metricsCollector) addRaftMembershipMetrics(membership raftx.Membership) {
	collector.gauge("maxio_raft_members", "Voting raft members.", len(membership.Nodes))
	collector.gauge("maxio_raft_removed_members", "Removed raft members.", len(membership.Removed))
	collector.gauge("maxio_raft_non_voting_members", "Non-voting raft members.", len(membership.NonVotings))
	collector.gauge("maxio_raft_witness_members", "Witness raft members.", len(membership.Witnesses))
	collector.gaugeUint64("maxio_raft_config_change_id", "Latest raft membership config change ID.", membership.ConfigChangeID)
}
