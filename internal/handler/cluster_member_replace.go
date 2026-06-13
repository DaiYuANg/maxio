package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/control"
)

type replacementReplicaRequest struct {
	ReplicaID uint64 `json:"replica_id"`
	Target    string `json:"target"`
}

type replacementReplicaResponse struct {
	OldReplicaID uint64 `json:"old_replica_id"`
	NewReplicaID uint64 `json:"new_replica_id"`
	Target       string `json:"target"`
	Objects      int    `json:"objects"`
	Shards       int    `json:"shards"`
	UsedBytes    int64  `json:"used_bytes"`
	Status       string `json:"status"`
}

var (
	errCannotReplaceLocalReplica    = errors.New("cannot replace local cluster replica")
	errClusterReplaceMemberNotFound = errors.New("cluster replace member not found")
)

func (s *Service) handleReplaceClusterMember(w http.ResponseWriter, r *http.Request, oldReplicaID uint64) {
	req, err := decodeReplacementReplicaRequest(r)
	if err != nil {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	result, err := s.replaceClusterMember(r.Context(), oldReplicaID, req)
	if err != nil {
		s.writeClusterReplaceError(w, err)
		return
	}
	s.auditHTTP(r, "cluster.member.replace", "old_replica_id", oldReplicaID, "new_replica_id", req.ReplicaID, "target", req.Target)
	s.writeJSON(w, http.StatusAccepted, result)
}

func (s *Service) writeClusterReplaceError(w http.ResponseWriter, err error) {
	if errors.Is(err, errCannotReplaceLocalReplica) {
		s.writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if errors.Is(err, errClusterReplaceMemberNotFound) {
		s.writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	s.writeError(w, err)
}

func decodeReplacementReplicaRequest(r *http.Request) (replacementReplicaRequest, error) {
	var req replacementReplicaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, fmt.Errorf("decode replacement replica request: %w", err)
	}
	req.Target = strings.TrimSpace(req.Target)
	if req.ReplicaID == 0 {
		return req, errors.New("replacement replica_id must be greater than zero")
	}
	if req.Target == "" {
		return req, errors.New("replacement target is required")
	}
	return req, nil
}

func (s *Service) replaceClusterMember(ctx context.Context, oldReplicaID uint64, req replacementReplicaRequest) (replacementReplicaResponse, error) {
	if s == nil || s.control == nil {
		return replacementReplicaResponse{}, errors.New("cluster replacement dependencies unavailable")
	}
	req, err := s.prepareClusterMemberReplacement(ctx, oldReplicaID, req)
	if err != nil {
		return replacementReplicaResponse{}, err
	}
	if err := s.runClusterMemberReplacement(ctx, oldReplicaID, req); err != nil {
		return replacementReplicaResponse{}, err
	}
	return replacementReplicaResponse{OldReplicaID: oldReplicaID, NewReplicaID: req.ReplicaID, Target: req.Target, Status: "replaced"}, nil
}

func (s *Service) prepareClusterMemberReplacement(ctx context.Context, oldReplicaID uint64, req replacementReplicaRequest) (replacementReplicaRequest, error) {
	membership, err := s.control.GetMembership(ctx)
	if err != nil {
		return req, fmt.Errorf("get control membership: %w", err)
	}
	if validationErr := ValidateClusterMemberReplacement(oldReplicaID, membership); validationErr != nil {
		return req, validationErr
	}
	replacementReplicaID, err := resolveReplacementReplicaID(oldReplicaID, req.ReplicaID, membership)
	if err != nil {
		return req, err
	}
	req.ReplicaID = replacementReplicaID
	return req, nil
}

func ValidateClusterMemberReplacement(oldReplicaID uint64, membership control.Membership) error {
	if oldReplicaID == 0 {
		return errors.New("old replica_id must be greater than zero")
	}
	if membership.LocalReplicaID == oldReplicaID {
		return errCannotReplaceLocalReplica
	}
	if _, ok := membership.Nodes[oldReplicaID]; !ok {
		return fmt.Errorf("%w: old replica %d", errClusterReplaceMemberNotFound, oldReplicaID)
	}
	return nil
}

func resolveReplacementReplicaID(oldReplicaID, requestedReplicaID uint64, membership control.Membership) (uint64, error) {
	if requestedReplicaID != oldReplicaID {
		return requestedReplicaID, nil
	}
	if len(membership.Nodes) == 0 {
		return oldReplicaID + 1, nil
	}
	maxReplicaID := maxMembershipReplicaID(oldReplicaID, membership)
	if maxReplicaID == math.MaxUint64 {
		return 0, errors.New("cannot auto-assign replacement replica_id: id space is exhausted")
	}
	usedIDs := buildReplicaIDSet(membership)
	for candidate := maxReplicaID + 1; ; candidate++ {
		if _, used := usedIDs[candidate]; !used {
			return candidate, nil
		}
		if candidate == math.MaxUint64 {
			return 0, errors.New("cannot auto-assign replacement replica_id: id space is exhausted")
		}
	}
}

func maxMembershipReplicaID(baseID uint64, membership control.Membership) uint64 {
	maxReplicaID := baseID
	for replicaID := range membership.Nodes {
		if replicaID > maxReplicaID {
			maxReplicaID = replicaID
		}
	}
	for _, replicaID := range membership.Removed {
		if replicaID > maxReplicaID {
			maxReplicaID = replicaID
		}
	}
	return maxReplicaID
}

func buildReplicaIDSet(membership control.Membership) map[uint64]struct{} {
	total := len(membership.Nodes) + len(membership.Removed)
	used := make(map[uint64]struct{}, total)
	for id := range membership.Nodes {
		used[id] = struct{}{}
	}
	for _, id := range membership.Removed {
		used[id] = struct{}{}
	}
	return used
}

func (s *Service) runClusterMemberReplacement(ctx context.Context, oldReplicaID uint64, req replacementReplicaRequest) error {
	if err := s.addReplacementReplica(ctx, req); err != nil {
		return err
	}
	return s.removeReplacedReplica(ctx, oldReplicaID)
}

func (s *Service) addReplacementReplica(ctx context.Context, req replacementReplicaRequest) error {
	if err := s.control.AddReplica(ctx, req.ReplicaID, req.Target); err != nil {
		return fmt.Errorf("add replacement replica: %w", err)
	}
	if err := s.syncStorageNodes(ctx); err != nil {
		return fmt.Errorf("sync storage nodes after replacement add: %w", err)
	}
	return nil
}

func (s *Service) removeReplacedReplica(ctx context.Context, oldReplicaID uint64) error {
	if err := s.ensureClusterMemberDecommissionable(ctx, oldReplicaID); err != nil {
		return fmt.Errorf("verify old replica decommission: %w", err)
	}
	if err := s.control.RemoveReplica(ctx, oldReplicaID); err != nil {
		return fmt.Errorf("remove old replica: %w", err)
	}
	if err := s.syncStorageNodes(ctx); err != nil {
		return fmt.Errorf("sync storage nodes after replacement remove: %w", err)
	}
	return nil
}
