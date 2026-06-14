package cache

func (cfg MemoryConfig) normalized() MemoryConfig {
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultMemoryTTL
	}
	if cfg.MaxCost <= 0 {
		cfg.MaxCost = DefaultMemoryMaxCost
	}
	if cfg.NumCounters <= 0 {
		cfg.NumCounters = DefaultMemoryNumCounters
	}
	if cfg.BufferItems <= 0 {
		cfg.BufferItems = DefaultMemoryBufferItems
	}
	return cfg
}
