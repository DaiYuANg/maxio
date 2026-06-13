package config

func Default() Config {
	return Config{
		HTTPAddress:                  ":8080",
		HTTPBodyLimit:                1 << 30,
		StorageAddress:               "127.0.0.1:8080",
		EnableClusterManagement:      false,
		EnableNativeObjectAPI:        false,
		MetadataBackend:              "sqlite",
		MetadataAutoMigrate:          true,
		EnableS3Proxy:                false,
		S3ProxyEntrypoint:            "",
		S3ProxyAdminAddress:          ":19090",
		S3ProxyHealthInterval:        "5s",
		S3ProxyHealthTimeout:         "2s",
		CacheBackend:                 "memory",
		CacheTTL:                     "1m",
		CacheMaxCost:                 100000,
		CacheRedisAddress:            "127.0.0.1:6379",
		CacheKeyPrefix:               "maxio",
		DataDir:                      "./data",
		LogLevel:                     "info",
		RaftNodeID:                   1,
		RaftShardID:                  1,
		RaftAddress:                  "127.0.0.1:63000",
		RaftDataDir:                  "raft",
		RaftBootstrap:                true,
		RaftOperationTimeout:         "5s",
		GossipBindAddress:            "0.0.0.0:7946",
		PendingObjectTTL:             "1h",
		RepairInterval:               "10m",
		RepairOnStart:                true,
		RepairMaxBatch:               100,
		RepairMaxRetries:             2,
		RepairRetryBackoff:           "1s",
		RepairRetryMaxBackoff:        "10s",
		RepairRetryBackoffMultiplier: 2,
		DedupeInterval:               "30m",
		DedupeOnStart:                true,
		DedupeMaxFixes:               100,
		IndexTimeout:                 "30s",
		IndexRetryBackoff:            "1s",
		IndexMaxRetries:              2,
		IndexQueueSize:               1024,
	}
}

func applyZeroDefaults(cfg Config) Config {
	cfg = applyRuntimeZeroDefaults(cfg)
	cfg = applyS3ProxyZeroDefaults(cfg)
	cfg = applyCacheZeroDefaults(cfg)
	cfg = applyRepairZeroDefaults(cfg)
	cfg = applyDedupeZeroDefaults(cfg)
	return applyIndexZeroDefaults(cfg)
}

func applyRuntimeZeroDefaults(cfg Config) Config {
	if cfg.MetadataBackend == "" {
		cfg.MetadataBackend = Default().MetadataBackend
	}
	if cfg.RaftNodeID == 0 {
		cfg.RaftNodeID = 1
	}
	if cfg.StorageAddress == "" {
		cfg.StorageAddress = storageAddressFromHTTPAddress(cfg.HTTPAddress)
	}
	if cfg.HTTPBodyLimit <= 0 {
		cfg.HTTPBodyLimit = Default().HTTPBodyLimit
	}
	if cfg.RaftShardID == 0 {
		cfg.RaftShardID = 1
	}
	if cfg.RaftDataDir == "" {
		cfg.RaftDataDir = "raft"
	}
	if cfg.PendingObjectTTL == "" {
		cfg.PendingObjectTTL = Default().PendingObjectTTL
	}
	if cfg.RaftOperationTimeout == "" {
		cfg.RaftOperationTimeout = Default().RaftOperationTimeout
	}
	if cfg.GossipBindAddress == "" {
		cfg.GossipBindAddress = Default().GossipBindAddress
	}
	return cfg
}

func applyCacheZeroDefaults(cfg Config) Config {
	if cfg.CacheBackend == "" {
		cfg.CacheBackend = Default().CacheBackend
	}
	if cfg.CacheTTL == "" {
		cfg.CacheTTL = Default().CacheTTL
	}
	if cfg.CacheMaxCost <= 0 {
		cfg.CacheMaxCost = Default().CacheMaxCost
	}
	if cfg.CacheRedisAddress == "" {
		cfg.CacheRedisAddress = Default().CacheRedisAddress
	}
	if cfg.CacheKeyPrefix == "" {
		cfg.CacheKeyPrefix = Default().CacheKeyPrefix
	}
	return cfg
}

func applyS3ProxyZeroDefaults(cfg Config) Config {
	if cfg.S3ProxyEntrypoint == "" {
		cfg.S3ProxyEntrypoint = Default().S3ProxyEntrypoint
	}
	if cfg.S3ProxyAdminAddress == "" {
		cfg.S3ProxyAdminAddress = Default().S3ProxyAdminAddress
	}
	if cfg.S3ProxyHealthInterval == "" {
		cfg.S3ProxyHealthInterval = Default().S3ProxyHealthInterval
	}
	if cfg.S3ProxyHealthTimeout == "" {
		cfg.S3ProxyHealthTimeout = Default().S3ProxyHealthTimeout
	}
	return cfg
}
