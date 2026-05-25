package engine

// ShardStore abstracts physical shard and layout persistence.
type ShardStore interface {
	ShardSize() int64
	WriteShard(shardDir, hash string, index int, data []byte) error
	WriteMeta(shardDir, layoutID string, data []byte) error
	ReadShard(shardDir, hash string, index int) ([]byte, error)
	ShardExists(shardDir, hash string, index int) bool
	DeleteShard(shardDir, hash string, index int) error
	ReadMeta(shardDir, layoutID string) ([]byte, error)
	DeleteShardSet(shardDir, hash string) error
	DeleteMeta(shardDir, layoutID string) error
	ListShards(shardDir, hash string) ([]int, error)
}
