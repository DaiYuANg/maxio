package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Layout maps object keys to their shard locations.
type Layout struct {
	ID              string
	ShardDir        string
	Hash            string
	Shards          []Shard
	ShardPlacements []ShardPlacement
	ShardChecksums  []string
	ShardSizes      []int64
	Bucket          string
	Key             string
	Size            int64
	ETag            string
	ShardSize       int64
	CoderType       string
	ContentType     string
	UpdatedAt       time.Time
	Version         int64
}

func (l Layout) String() string {
	return fmt.Sprintf("layout{%s:%s,shards=%d,hash=%s}", l.Bucket, l.Key, len(l.Shards), l.Hash)
}

func layoutKey(bucket, key string) string {
	return bucket + "/" + key
}

func layoutHash(key string) string {
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:16]) // use first 16 bytes as hash
}

func makeShardDir(prefix string) string {
	if len(prefix) < 2 {
		return strings.ToLower(prefix)
	}
	return strings.ToLower(prefix[:2])
}

// ShardDirForKey returns the deterministic shard directory used for an object key.
func ShardDirForKey(key string) string {
	return makeShardDir(key)
}

func cloneStrings(input []string) []string {
	if len(input) == 0 {
		return nil
	}
	output := make([]string, len(input))
	copy(output, input)
	return output
}

func cloneInt64s(input []int64) []int64 {
	if len(input) == 0 {
		return nil
	}
	output := make([]int64, len(input))
	copy(output, input)
	return output
}
