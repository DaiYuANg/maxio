package discovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/memberlist"
	"github.com/lyonbrown4d/maxio/internal/config"
)

type Node struct {
	Name           string `json:"name"`
	Address        string `json:"address"`
	State          string `json:"state"`
	ReplicaID      uint64 `json:"replica_id"`
	ControlAddress string `json:"control_address"`
	HTTPAddress    string `json:"http_address"`
}

type Runtime struct {
	cfg    config.Config
	logger *slog.Logger

	mu      sync.RWMutex
	list    *memberlist.Memberlist
	meta    nodeMeta
	seeds   []string
	started bool
}

type nodeMeta struct {
	Version        int    `json:"version"`
	ReplicaID      uint64 `json:"replica_id"`
	ControlAddress string `json:"control_address"`
	HTTPAddress    string `json:"http_address"`
}

func NewRuntime(cfg config.Config, logger *slog.Logger) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	return &Runtime{
		cfg:    cfg,
		logger: logger,
		meta: nodeMeta{
			Version:        1,
			ReplicaID:      cfg.NodeID,
			ControlAddress: cfg.StorageAdvertiseAddress(),
			HTTPAddress:    cfg.StorageAdvertiseAddress(),
		},
		seeds: splitSeeds(cfg.GossipSeeds),
	}
}

func (rt *Runtime) Start(ctx context.Context) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.started {
		return nil
	}
	memberlistCfg, err := rt.memberlistConfig()
	if err != nil {
		return err
	}
	list, err := memberlist.Create(memberlistCfg)
	if err != nil {
		return fmt.Errorf("create memberlist: %w", err)
	}
	rt.list = list
	rt.started = true

	joined, err := rt.joinSeeds()
	if err != nil {
		rt.logger.WarnContext(ctx, "discovery seed join failed", "error", err, "seeds", rt.seeds)
	}
	rt.logger.InfoContext(ctx, "discovery started",
		"node", memberlistCfg.Name,
		"bind", rt.cfg.GossipBindAddress,
		"joined", joined,
	)
	return nil
}

func (rt *Runtime) Stop(ctx context.Context) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if rt.list == nil {
		return nil
	}
	if err := rt.list.Leave(time.Second); err != nil {
		rt.logger.WarnContext(ctx, "discovery leave failed", "error", err)
	}
	if err := rt.list.Shutdown(); err != nil {
		return fmt.Errorf("shutdown memberlist: %w", err)
	}
	rt.list = nil
	rt.started = false
	return nil
}

func (rt *Runtime) Nodes() []Node {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if rt.list == nil {
		return []Node{}
	}
	members := rt.list.Members()
	nodes := make([]Node, 0, len(members))
	for _, member := range members {
		nodes = append(nodes, nodeFromMember(member))
	}
	return nodes
}

func (rt *Runtime) memberlistConfig() (*memberlist.Config, error) {
	host, port, err := splitHostPort(rt.cfg.GossipBindAddress)
	if err != nil {
		return nil, fmt.Errorf("parse gossip bind address: %w", err)
	}
	cfg := memberlist.DefaultLANConfig()
	cfg.Name = fmt.Sprintf("maxio-%d", rt.cfg.NodeID)
	cfg.BindAddr = host
	cfg.BindPort = port
	cfg.Delegate = &delegate{meta: rt.meta}
	cfg.LogOutput = io.Discard
	if rt.cfg.GossipAdvertiseAddress != "" {
		advertiseHost, advertisePort, err := splitHostPort(rt.cfg.GossipAdvertiseAddress)
		if err != nil {
			return nil, fmt.Errorf("parse gossip advertise address: %w", err)
		}
		cfg.AdvertiseAddr = advertiseHost
		cfg.AdvertisePort = advertisePort
	}
	return cfg, nil
}

func (rt *Runtime) joinSeeds() (int, error) {
	if len(rt.seeds) == 0 || rt.list == nil {
		return 0, nil
	}
	joined, err := rt.list.Join(rt.seeds)
	if err != nil {
		return 0, fmt.Errorf("join discovery seeds: %w", err)
	}
	return joined, nil
}

func nodeFromMember(member *memberlist.Node) Node {
	if member == nil {
		return Node{}
	}
	meta := decodeNodeMeta(member.Meta)
	return Node{
		Name:           member.Name,
		Address:        member.Address(),
		State:          nodeStateName(member.State),
		ReplicaID:      meta.ReplicaID,
		ControlAddress: meta.ControlAddress,
		HTTPAddress:    meta.HTTPAddress,
	}
}

func decodeNodeMeta(data []byte) nodeMeta {
	var meta nodeMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nodeMeta{}
	}
	return meta
}

func nodeStateName(state memberlist.NodeStateType) string {
	switch state {
	case memberlist.StateAlive:
		return "alive"
	case memberlist.StateSuspect:
		return "suspect"
	case memberlist.StateDead:
		return "dead"
	case memberlist.StateLeft:
		return "left"
	default:
		return "unknown"
	}
}

func splitHostPort(value string) (string, int, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return "", 0, fmt.Errorf("split host port: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return "", 0, fmt.Errorf("parse port: %w", err)
	}
	if port <= 0 || port > 65535 {
		return "", 0, errors.New("port must be between 1 and 65535")
	}
	return host, port, nil
}

func splitSeeds(value string) []string {
	parts := strings.Split(value, ",")
	seeds := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			seeds = append(seeds, part)
		}
	}
	return seeds
}
