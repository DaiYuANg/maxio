package handler

import (
	"context"
	"errors"

	"github.com/lyonbrown4d/maxio/internal/control"
)

func (collector *metricsCollector) addControlStatus(ctx context.Context, s *Service) {
	if s == nil || s.control == nil {
		collector.gaugeUint64("maxio_control_local_replica_id", "Local cluster replica ID.", 0)
		collector.gauge("maxio_control_leader_available", "Whether a control leader is currently known.", 0)
		collector.gauge("maxio_control_local_is_leader", "Whether the local cluster replica is the current leader.", 0)
		collector.addControlMembershipMetrics(control.Membership{})
		return
	}
	collector.gaugeUint64("maxio_control_local_replica_id", "Local cluster replica ID.", s.control.LocalReplicaID())
	collector.addControlLeaderStatus(s.control.AssertLeader(ctx))

	membership, err := s.control.GetMembership(ctx)
	if err != nil {
		collector.collectionErrors++
		return
	}
	collector.addControlMembershipMetrics(membership)
}

func (collector *metricsCollector) addControlLeaderStatus(err error) {
	collector.gauge("maxio_control_leader_available", "Whether a control leader is currently known.", boolInt(!errors.Is(err, control.ErrLeaderUnavailable)))
	collector.gauge("maxio_control_local_is_leader", "Whether the local cluster replica is the current leader.", boolInt(err == nil))
}

func (collector *metricsCollector) addControlMembershipMetrics(membership control.Membership) {
	collector.gauge("maxio_control_members", "Control-plane members.", len(membership.Nodes))
	collector.gauge("maxio_control_removed_members", "Removed control-plane members.", len(membership.Removed))
	collector.gauge("maxio_control_non_voting_members", "Non-voting control-plane members.", len(membership.NonVotings))
	collector.gauge("maxio_control_witness_members", "Witness control-plane members.", len(membership.Witnesses))
	collector.gaugeUint64("maxio_control_config_change_id", "Latest control membership config change ID.", membership.ConfigChangeID)
}
