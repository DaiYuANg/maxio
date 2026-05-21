package engine_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/lyonbrown4d/maxio/internal/engine"
	"github.com/spf13/afero"
)

type testShardHTTPStorage struct {
	clusterValue string
	mu           sync.RWMutex
	shards       map[string][]byte
}

func newTestShardHTTPStorage() *testShardHTTPStorage {
	return &testShardHTTPStorage{
		shards: map[string][]byte{},
	}
}

func (s *testShardHTTPStorage) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.clusterValue != "" && r.Header.Get("X-Maxio-Cluster") != s.clusterValue {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	shardDir, hash, index, err := parseShardRequestPath(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodPut:
		s.handleWrite(w, r, shardDir, hash, index)
	case http.MethodGet:
		s.handleRead(w, r, shardDir, hash, index)
	case http.MethodHead:
		s.handleHead(w, r, shardDir, hash, index)
	case http.MethodDelete:
		s.handleDelete(w, r, shardDir, hash, index)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func parseShardRequestPath(path string) (string, string, int, error) {
	route := strings.Trim(path, "/")
	if !strings.HasPrefix(route, "_internal/storage/shards/") {
		return "", "", 0, errors.New("invalid path")
	}
	parts := strings.Split(strings.TrimPrefix(route, "_internal/storage/shards/"), "/")
	if len(parts) != 3 {
		return "", "", 0, errors.New("invalid path")
	}

	shardDir, err := url.PathUnescape(parts[0])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid shard directory path: %w", err)
	}
	hash, err := url.PathUnescape(parts[1])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid shard hash path: %w", err)
	}
	index, err := strconv.Atoi(parts[2])
	if err != nil {
		return "", "", 0, fmt.Errorf("invalid shard index path: %w", err)
	}
	return shardDir, hash, index, nil
}

func (s *testShardHTTPStorage) handleWrite(w http.ResponseWriter, r *http.Request, shardDir, hash string, index int) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	s.shards[shardKey(shardDir, hash, index)] = append([]byte(nil), data...)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *testShardHTTPStorage) handleRead(w http.ResponseWriter, r *http.Request, shardDir, hash string, index int) {
	s.mu.RLock()
	data, ok := s.shards[shardKey(shardDir, hash, index)]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	if _, writeErr := w.Write(data); writeErr != nil {
		return
	}
}

func (s *testShardHTTPStorage) handleHead(w http.ResponseWriter, r *http.Request, shardDir, hash string, index int) {
	s.mu.RLock()
	_, ok := s.shards[shardKey(shardDir, hash, index)]
	s.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *testShardHTTPStorage) handleDelete(w http.ResponseWriter, _ *http.Request, shardDir, hash string, index int) {
	s.mu.Lock()
	delete(s.shards, shardKey(shardDir, hash, index))
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func shardKey(shardDir, hash string, index int) string {
	return shardDir + "|" + hash + "|" + strconv.Itoa(index)
}

func TestRemoteStorageNodeWritesAndReadsShardsViaHTTP(t *testing.T) {
	ctx := context.Background()
	storage := newTestShardHTTPStorage()
	server := httptest.NewServer(storage)
	defer server.Close()

	e := newTestEngineForRemote(t, server.URL)
	node, err := e.StorageNode("raft-2")
	if err != nil {
		t.Fatalf("StorageNode: %v", err)
	}

	data := []byte("distributed shard payload")
	if writeErr := node.WriteShard(ctx, "ab", "hash-1", 3, data); writeErr != nil {
		t.Fatalf("write remote shard: %v", writeErr)
	}

	exists := node.ShardExists(ctx, "ab", "hash-1", 3)
	if !exists {
		t.Fatal("expected remote shard exists after write")
	}

	got, err := node.ReadShard(ctx, "ab", "hash-1", 3)
	if err != nil {
		t.Fatalf("read remote shard: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("read shard = %q, want %q", got, data)
	}
	if deleteErr := node.DeleteShard(ctx, "ab", "hash-1", 3); deleteErr != nil {
		t.Fatalf("delete remote shard: %v", deleteErr)
	}
	if exists := node.ShardExists(ctx, "ab", "hash-1", 3); exists {
		t.Fatal("expected remote shard deleted")
	}
}

func TestRemoteStorageNodeReadMissingReturnsErrNotExist(t *testing.T) {
	ctx := context.Background()
	storage := newTestShardHTTPStorage()
	server := httptest.NewServer(storage)
	defer server.Close()

	e := newTestEngineForRemote(t, server.URL)
	node, err := e.StorageNode("raft-2")
	if err != nil {
		t.Fatalf("StorageNode: %v", err)
	}

	_, err = node.ReadShard(ctx, "ab", "hash-missing", 7)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read missing shard error = %v, want os.ErrNotExist", err)
	}
	if exists := node.ShardExists(ctx, "ab", "hash-missing", 7); exists {
		t.Fatal("expected missing shard to not exist")
	}
}

func TestRemoteStorageNodeSendsClusterHeaderViaHTTP(t *testing.T) {
	ctx := context.Background()
	storage := newTestShardHTTPStorage()
	storage.clusterValue = "cluster-secret"
	server := httptest.NewServer(storage)
	defer server.Close()

	e := newTestEngineForRemoteWithToken(t, server.URL, storage.clusterValue)
	node, err := e.StorageNode("raft-2")
	if err != nil {
		t.Fatalf("StorageNode: %v", err)
	}

	if err := node.WriteShard(ctx, "ab", "hash-auth", 1, []byte("payload")); err != nil {
		t.Fatalf("write remote shard with cluster header: %v", err)
	}
}

func newTestEngineForRemote(t *testing.T, remoteAddress string) *engine.Engine {
	t.Helper()
	return newTestEngineForRemoteWithToken(t, remoteAddress, "")
}

func newTestEngineForRemoteWithToken(t *testing.T, remoteAddress, controlValue string) *engine.Engine {
	t.Helper()
	e, err := engine.NewEngine(t.TempDir(), engine.DefaultDataChunks, engine.DefaultParityChunks, afero.NewMemMapFs())
	if err != nil {
		t.Fatalf("create test engine: %v", err)
	}
	e.SetControlToken(controlValue)
	if err := e.SyncStorageNodesFromRaft(1, map[uint64]string{
		1: "127.0.0.1:9000",
		2: remoteAddress,
	}); err != nil {
		t.Fatalf("sync storage nodes: %v", err)
	}
	return e
}
