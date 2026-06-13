package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type bootstrapClusterRequest struct {
	Nodes map[uint64]string `json:"nodes"`
}

func decodeClusterNodeMap(r *http.Request) (map[uint64]string, error) {
	var req bootstrapClusterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("decode cluster nodes: %w", err)
	}
	return normalizeClusterNodes(req.Nodes)
}

func decodeAddReplicaRequest(r *http.Request, operation string) (addReplicaRequest, error) {
	var req addReplicaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, fmt.Errorf("decode %s request: %w", operation, err)
	}
	req.Target = strings.TrimSpace(req.Target)
	if req.ReplicaID == 0 {
		return req, fmt.Errorf("%s replica_id must be greater than zero", operation)
	}
	if req.Target == "" {
		return req, fmt.Errorf("%s target is required", operation)
	}
	return req, nil
}

func normalizeClusterNodes(input map[uint64]string) (map[uint64]string, error) {
	if len(input) == 0 {
		return nil, errors.New("nodes are required")
	}
	nodes := make(map[uint64]string, len(input))
	for replicaID, target := range input {
		target = strings.TrimSpace(target)
		if replicaID == 0 {
			return nil, errors.New("replica_id must be greater than zero")
		}
		if target == "" {
			return nil, fmt.Errorf("node %d target is required", replicaID)
		}
		nodes[replicaID] = target
	}
	return nodes, nil
}

func parseReplicaIDSegment(raw string) (uint64, error) {
	if raw == "" {
		return 0, errors.New("replica_id is required")
	}
	replicaID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse replica_id: %w", err)
	}
	if replicaID == 0 {
		return 0, errors.New("replica_id must be greater than zero")
	}
	return replicaID, nil
}

func parseRequiredReplicaID(r *http.Request) (uint64, error) {
	return parseReplicaIDSegment(r.URL.Query().Get("replica_id"))
}
