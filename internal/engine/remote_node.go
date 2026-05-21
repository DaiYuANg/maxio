package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	internalStorageShardsPath = "/_internal/storage/shards"
	maxioClusterHeader        = "X-Maxio-Cluster"
)

type remoteStorageNode struct {
	id           string
	address      string
	baseURL      string
	controlToken string
	client       *http.Client
}

func newRemoteStorageNode(id, address string, client *http.Client) (*remoteStorageNode, error) {
	id = strings.TrimSpace(id)
	address = strings.TrimSpace(address)
	if id == "" {
		return nil, errors.New("storage node id is required")
	}
	if address == "" {
		return nil, errors.New("storage node address is required")
	}

	baseURL, err := normalizeStorageNodeAddress(address)
	if err != nil {
		return nil, fmt.Errorf("normalize storage node address: %w", err)
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &remoteStorageNode{
		id:      id,
		address: address,
		baseURL: baseURL,
		client:  client,
	}, nil
}

func NewRemoteStorageNode(id, address string, client *http.Client) (*remoteStorageNode, error) {
	return newRemoteStorageNode(id, address, client)
}

func (node *remoteStorageNode) ID() string {
	if node == nil || strings.TrimSpace(node.id) == "" {
		return DefaultLocalNodeID
	}
	return strings.TrimSpace(node.id)
}

func (node *remoteStorageNode) Address() string {
	if node == nil || strings.TrimSpace(node.address) == "" {
		return DefaultLocalNodeAddress
	}
	return strings.TrimSpace(node.address)
}

func (node *remoteStorageNode) WriteShard(ctx context.Context, shardDir, hash string, index int, data []byte) error {
	if err := contextError(ctx, "write remote shard"); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, node.shardURL(shardDir, hash, index), bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build remote shard write request: %w", err)
	}
	node.authorizeRequest(req)
	resp, err := node.client.Do(req)
	if err != nil {
		return fmt.Errorf("send remote shard write request: %w", err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if closeErr := closeResponseBody(resp); closeErr != nil {
			return fmt.Errorf("close remote shard write response: %w", closeErr)
		}
		return nil
	}
	responseBody, err := readAndCloseResponseBody(resp)
	if err != nil {
		return fmt.Errorf("remote shard write request failed: %w", err)
	}
	return fmt.Errorf("remote shard write request failed: status=%d body=%q", resp.StatusCode, strings.TrimSpace(string(responseBody)))
}

func (node *remoteStorageNode) ReadShard(ctx context.Context, shardDir, hash string, index int) ([]byte, error) {
	if err := contextError(ctx, "read remote shard"); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, node.shardURL(shardDir, hash, index), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build remote shard read request: %w", err)
	}
	node.authorizeRequest(req)
	resp, err := node.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send remote shard read request: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		if closeErr := closeResponseBody(resp); closeErr != nil {
			return nil, fmt.Errorf("close remote shard read response: %w", closeErr)
		}
		return nil, fmt.Errorf("remote shard missing: %w", os.ErrNotExist)
	case http.StatusOK:
		return readAndCloseResponseBody(resp)
	default:
		responseBody, err := readAndCloseResponseBody(resp)
		if err != nil {
			return nil, fmt.Errorf("read remote shard error response: %w", err)
		}
		return nil, fmt.Errorf("remote shard read request failed: status=%d body=%q", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
}

func (node *remoteStorageNode) ShardExists(ctx context.Context, shardDir, hash string, index int) bool {
	if err := contextError(ctx, "check remote shard"); err != nil {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, node.shardURL(shardDir, hash, index), http.NoBody)
	if err != nil {
		return false
	}
	node.authorizeRequest(req)
	resp, err := node.client.Do(req)
	if err != nil {
		return false
	}
	if err := closeResponseBody(resp); err != nil {
		return false
	}
	return resp.StatusCode == http.StatusOK
}

func (node *remoteStorageNode) DeleteShard(ctx context.Context, shardDir, hash string, index int) error {
	if err := contextError(ctx, "delete remote shard"); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, node.shardURL(shardDir, hash, index), http.NoBody)
	if err != nil {
		return fmt.Errorf("build remote shard delete request: %w", err)
	}
	node.authorizeRequest(req)
	resp, err := node.client.Do(req)
	if err != nil {
		return fmt.Errorf("send remote shard delete request: %w", err)
	}
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotFound {
		if closeErr := closeResponseBody(resp); closeErr != nil {
			return fmt.Errorf("close remote shard delete response: %w", closeErr)
		}
		return nil
	}
	responseBody, err := readAndCloseResponseBody(resp)
	if err != nil {
		return fmt.Errorf("remote shard delete request failed: %w", err)
	}
	return fmt.Errorf("remote shard delete request failed: status=%d body=%q", resp.StatusCode, strings.TrimSpace(string(responseBody)))
}

func (node *remoteStorageNode) authorizeRequest(req *http.Request) {
	if node == nil || req == nil || strings.TrimSpace(node.controlToken) == "" {
		return
	}
	req.Header.Set(maxioClusterHeader, strings.TrimSpace(node.controlToken))
}

func (node *remoteStorageNode) shardURL(shardDir, hash string, index int) string {
	return strings.TrimRight(node.baseURL, "/") + internalStorageShardsPath + "/" +
		url.PathEscape(shardDir) + "/" +
		url.PathEscape(hash) + "/" +
		strconv.Itoa(index)
}

func normalizeStorageNodeAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("storage node address is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse storage node address: %w", err)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("storage node address missing host: %q", raw)
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	return responseBody, nil
}

func closeResponseBody(resp *http.Response) error {
	if err := resp.Body.Close(); err != nil {
		return fmt.Errorf("close response body: %w", err)
	}
	return nil
}

func readAndCloseResponseBody(resp *http.Response) ([]byte, error) {
	responseBody, err := readResponseBody(resp)
	closeErr := closeResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close response body: %w", closeErr)
	}
	return responseBody, nil
}
