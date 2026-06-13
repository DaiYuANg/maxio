// Package control defines the small runtime-control contracts shared by
// management handlers. It intentionally contains no embedded consensus runtime.
package control

import "errors"

var (
	ErrLeaderUnavailable = errors.New("control: leader unavailable")
	ErrNotLeader         = errors.New("control: local node is not leader")
)

type Replica struct {
	ReplicaID uint64 `json:"replica_id"`
	Target    string `json:"target"`
}

type Membership struct {
	Nodes          map[uint64]string `json:"nodes"`
	NonVotings     map[uint64]string `json:"non_votings,omitempty"`
	Witnesses      map[uint64]string `json:"witnesses,omitempty"`
	Removed        []uint64          `json:"removed,omitempty"`
	ConfigChangeID uint64            `json:"config_change_id"`
	LocalReplicaID uint64            `json:"local_replica_id"`
}

type SyncMembershipResult struct {
	Before Membership `json:"before"`
	After  Membership `json:"after"`
}
