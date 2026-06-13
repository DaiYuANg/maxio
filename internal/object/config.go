package object

import "time"

const (
	defaultDedupeMaxFixes    = 100
	defaultPendingObjectTTL  = time.Hour
	defaultIndexTimeout      = 30 * time.Second
	defaultIndexRetryBackoff = time.Second
	defaultIndexMaxRetries   = 2
	defaultIndexQueueSize    = 1024
)

type Config struct {
	DedupeMaxFixes    int
	PendingObjectTTL  time.Duration
	IndexTimeout      time.Duration
	IndexRetryBackoff time.Duration
	IndexMaxRetries   int
	IndexQueueSize    int
	IndexRateLimit    int
}

func (cfg Config) normalized() Config {
	if cfg.DedupeMaxFixes <= 0 {
		cfg.DedupeMaxFixes = defaultDedupeMaxFixes
	}
	if cfg.PendingObjectTTL <= 0 {
		cfg.PendingObjectTTL = defaultPendingObjectTTL
	}
	if cfg.IndexTimeout <= 0 {
		cfg.IndexTimeout = defaultIndexTimeout
	}
	if cfg.IndexRetryBackoff <= 0 {
		cfg.IndexRetryBackoff = defaultIndexRetryBackoff
	}
	if cfg.IndexMaxRetries <= 0 {
		cfg.IndexMaxRetries = defaultIndexMaxRetries
	}
	if cfg.IndexQueueSize <= 0 {
		cfg.IndexQueueSize = defaultIndexQueueSize
	}
	return cfg
}

func (cfg Config) PendingObjectTTLDuration() time.Duration {
	if cfg.PendingObjectTTL <= 0 {
		return defaultPendingObjectTTL
	}
	return cfg.PendingObjectTTL
}

func (cfg Config) IndexTimeoutDuration() time.Duration {
	if cfg.IndexTimeout <= 0 {
		return defaultIndexTimeout
	}
	return cfg.IndexTimeout
}

func (cfg Config) IndexRetryBackoffDuration() time.Duration {
	if cfg.IndexRetryBackoff <= 0 {
		return defaultIndexRetryBackoff
	}
	return cfg.IndexRetryBackoff
}
