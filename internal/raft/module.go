package raft

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/arcgolabs/dix"
	dragonboat "github.com/lni/dragonboat/v4"
	dcfg "github.com/lni/dragonboat/v4/config"
	dbsm "github.com/lni/dragonboat/v4/statemachine"
	"github.com/lyonbrown4d/maxio/internal/config"
)

type runtimeConfig struct {
	shardID          uint64
	replicaID        uint64
	raftAddress      string
	nodeHostDir      string
	bootstrap        bool
	join             bool
	initialMembers   map[uint64]dragonboat.Target
	rttMs            uint64
	electionRTT      uint64
	heartbeatRTT     uint64
	deploymentID     uint64
	operationTimeout time.Duration
}

type Runtime struct {
	cfg    *runtimeConfig
	node   *dragonboat.NodeHost
	logger *slog.Logger
}

var (
	ErrLeaderUnavailable = errors.New("raft: leader unavailable")
	ErrNotLeader         = errors.New("raft: local replica is not leader")
)

func Module() dix.Module {
	return dix.NewModule(
		"raft",
		dix.WithModuleProviders(
			dix.ProviderErr2(newRuntime),
		),
		dix.Hooks(
			dix.OnStart(startReplica),
			dix.OnStop(stopReplica),
		),
	)
}

func newRuntime(cfg config.Config, logger *slog.Logger) (*Runtime, error) {
	if !cfg.EnableClusterManagement {
		return &Runtime{}, nil
	}
	configureDragonboatLogger(logger)

	rtCfg, err := newRuntimeConfig(cfg)
	if err != nil {
		return nil, err
	}
	nodeHost, err := newNodeHost(rtCfg)
	if err != nil {
		return nil, err
	}
	return &Runtime{
		cfg:    rtCfg,
		node:   nodeHost,
		logger: logger,
	}, nil
}

func newRuntimeConfig(cfg config.Config) (*runtimeConfig, error) {
	initialMembers, err := parseInitialMembers(cfg.RaftInitialMembers)
	if err != nil {
		return nil, err
	}
	rtCfg := &runtimeConfig{
		raftAddress:      cfg.RaftAddress,
		shardID:          cfg.RaftShardID,
		replicaID:        cfg.RaftNodeID,
		nodeHostDir:      nodeHostDir(cfg),
		bootstrap:        cfg.RaftBootstrap,
		join:             cfg.RaftJoin,
		initialMembers:   initialMembers,
		rttMs:            200,
		electionRTT:      20,
		heartbeatRTT:     2,
		operationTimeout: cfg.RaftOperationTimeoutDuration(),
	}
	rtCfg.applyDefaults()
	if err := rtCfg.applyStartupMode(); err != nil {
		return nil, err
	}
	if validateErr := validateInitialMembers(rtCfg); validateErr != nil {
		return nil, validateErr
	}
	return rtCfg, nil
}

func nodeHostDir(cfg config.Config) string {
	if cfg.RaftDataDir != "" {
		return cfg.RaftDataDir
	}
	return filepath.Join(cfg.DataDir, "raft")
}

func (cfg *runtimeConfig) applyDefaults() {
	if cfg.shardID == 0 {
		cfg.shardID = 1
	}
	if cfg.replicaID == 0 {
		cfg.replicaID = 1
	}
}

func newNodeHost(cfg *runtimeConfig) (*dragonboat.NodeHost, error) {
	nhc := dcfg.NodeHostConfig{
		NodeHostDir:    cfg.nodeHostDir,
		WALDir:         cfg.nodeHostDir,
		RTTMillisecond: cfg.rttMs,
		RaftAddress:    cfg.raftAddress,
		DeploymentID:   cfg.deploymentID,
	}
	nodeHost, err := dragonboat.NewNodeHost(nhc)
	if err != nil {
		return nil, fmt.Errorf("create dragonboat nodehost: %w", err)
	}
	return nodeHost, nil
}

func startReplica(ctx context.Context, rt *Runtime) error {
	if rt == nil || rt.node == nil {
		return nil
	}
	create := func(_, _ uint64) dbsm.IStateMachine {
		return newRaftStateMachine(rt.cfg.shardID, rt.cfg.replicaID)
	}
	if err := rt.node.StartReplica(rt.startupMembers(), rt.cfg.join, create, rt.replicaConfig()); err != nil {
		return fmt.Errorf("start raft replica: %w", err)
	}
	rt.logStarted(ctx)
	return nil
}

func stopReplica(_ context.Context, rt *Runtime) error {
	if rt == nil || rt.node == nil {
		return nil
	}
	if err := rt.node.StopShard(rt.cfg.shardID); err != nil && rt.logger != nil {
		rt.logger.Warn("stop raft shard failed", "error", err)
	}
	rt.node.Close()
	return nil
}

func (rt *Runtime) replicaConfig() dcfg.Config {
	return dcfg.Config{
		ReplicaID:    rt.cfg.replicaID,
		ShardID:      rt.cfg.shardID,
		HeartbeatRTT: rt.cfg.heartbeatRTT,
		ElectionRTT:  rt.cfg.electionRTT,
		CheckQuorum:  true,
	}
}

func (rt *Runtime) logStarted(ctx context.Context) {
	if rt.logger == nil {
		return
	}
	members := rt.startupMembers()
	rt.logger.InfoContext(ctx, "dragonboat replica started",
		"mode", rt.startupMode(),
		"raft_address", rt.cfg.raftAddress,
		"shard_id", rt.cfg.shardID,
		"replica_id", rt.cfg.replicaID,
		"initial_members", len(members),
	)
}

func (rt *Runtime) NodeHost() *dragonboat.NodeHost {
	if rt == nil {
		return nil
	}
	return rt.node
}

func (rt *Runtime) AssertLeader(ctx context.Context) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("raft leader context: %w", err)
		}
	}
	if rt == nil || rt.cfg == nil || rt.node == nil {
		return ErrLeaderUnavailable
	}
	leaderID, _, valid, err := rt.node.GetLeaderID(rt.cfg.shardID)
	if err != nil {
		return fmt.Errorf("get raft leader: %w", err)
	}
	if !valid || leaderID == 0 {
		return ErrLeaderUnavailable
	}
	if leaderID != rt.cfg.replicaID {
		return fmt.Errorf("%w: leader=%d local=%d", ErrNotLeader, leaderID, rt.cfg.replicaID)
	}
	return nil
}

func (rt *Runtime) startupMembers() map[uint64]dragonboat.Target {
	if rt == nil || rt.cfg == nil || rt.cfg.join || !rt.cfg.bootstrap {
		return map[uint64]dragonboat.Target{}
	}
	if len(rt.cfg.initialMembers) == 0 {
		return map[uint64]dragonboat.Target{
			rt.cfg.replicaID: rt.cfg.raftAddress,
		}
	}
	return maps.Clone(rt.cfg.initialMembers)
}

func (rt *Runtime) startupMode() string {
	if rt == nil || rt.cfg == nil {
		return "unknown"
	}
	if rt.cfg.join {
		return "join"
	}
	if rt.cfg.bootstrap {
		return "bootstrap"
	}
	return "restart"
}

func parseInitialMembers(value string) (map[uint64]dragonboat.Target, error) {
	value = strings.TrimSpace(value)
	members := make(map[uint64]dragonboat.Target)
	if value == "" {
		return members, nil
	}
	for part := range strings.SplitSeq(value, ",") {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		replicaID, target, err := parseInitialMember(item)
		if err != nil {
			return nil, err
		}
		if _, exists := members[replicaID]; exists {
			return nil, fmt.Errorf("duplicate raft initial member: %d", replicaID)
		}
		members[replicaID] = target
	}
	return members, nil
}

func parseInitialMember(value string) (uint64, dragonboat.Target, error) {
	separator := strings.Index(value, "=")
	if separator < 0 {
		separator = strings.Index(value, "@")
	}
	if separator <= 0 || separator >= len(value)-1 {
		return 0, "", fmt.Errorf("invalid raft initial member %q, want id=address", value)
	}
	replicaID, err := strconv.ParseUint(strings.TrimSpace(value[:separator]), 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("parse raft initial member id: %w", err)
	}
	target := strings.TrimSpace(value[separator+1:])
	if replicaID == 0 {
		return 0, "", errors.New("raft initial member id must be greater than zero")
	}
	if target == "" {
		return 0, "", errors.New("raft initial member address is required")
	}
	return replicaID, target, nil
}

func validateInitialMembers(cfg *runtimeConfig) error {
	if cfg == nil || cfg.join || !cfg.bootstrap || len(cfg.initialMembers) == 0 {
		return nil
	}
	if _, ok := cfg.initialMembers[cfg.replicaID]; !ok {
		return fmt.Errorf("raft initial members must include local replica %d", cfg.replicaID)
	}
	return nil
}
