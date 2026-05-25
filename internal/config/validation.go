package config

import (
	"fmt"
	"time"
)

type durationConfig struct {
	name  string
	value string
}

type intConfig struct {
	name    string
	value   int
	minimum int
}

func validateDurations(cfg Config) error {
	durationConfigs := []durationConfig{
		{name: "raft_operation_timeout", value: cfg.RaftOperationTimeout},
		{name: "pending_object_ttl", value: cfg.PendingObjectTTL},
		{name: "repair_interval", value: cfg.RepairInterval},
		{name: "repair_retry_backoff", value: cfg.RepairRetryBackoff},
		{name: "repair_retry_max_backoff", value: cfg.RepairRetryMaxBackoff},
		{name: "dedupe_interval", value: cfg.DedupeInterval},
		{name: "index_timeout", value: cfg.IndexTimeout},
		{name: "index_retry_backoff", value: cfg.IndexRetryBackoff},
		{name: "cache_ttl", value: cfg.CacheTTL},
	}
	for _, cfgValue := range durationConfigs {
		if err := validateDuration(cfgValue.name, cfgValue.value); err != nil {
			return err
		}
	}

	integerConfigs := []intConfig{
		{name: "repair_max_retries", value: cfg.RepairMaxRetries, minimum: 0},
		{name: "repair_rate_limit", value: cfg.RepairRateLimit, minimum: 0},
		{name: "dedupe_max_fixes", value: cfg.DedupeMaxFixes, minimum: 0},
		{name: "index_max_retries", value: cfg.IndexMaxRetries, minimum: 0},
		{name: "index_queue_size", value: cfg.IndexQueueSize, minimum: 0},
		{name: "index_rate_limit", value: cfg.IndexRateLimit, minimum: 0},
		{name: "cache_max_cost", value: cfg.CacheMaxCost, minimum: 0},
		{name: "cache_redis_db", value: cfg.CacheRedisDB, minimum: 0},
	}
	for _, cfgValue := range integerConfigs {
		if err := validateNonNegativeInt(cfgValue.name, cfgValue.value, cfgValue.minimum); err != nil {
			return err
		}
	}

	if err := validateMultiplier("repair_retry_multiplier", cfg.RepairRetryBackoffMultiplier); err != nil {
		return err
	}
	return validateIndexConfig(cfg)
}

func validateDuration(name, value string) error {
	if _, err := time.ParseDuration(value); err != nil {
		return fmt.Errorf("invalid config: %s: %w", name, err)
	}
	return nil
}

func validateNonNegativeInt(name string, value, minimum int) error {
	if value < minimum {
		if minimum == 0 {
			return fmt.Errorf("invalid config: %s must be non-negative", name)
		}
		return fmt.Errorf("invalid config: %s must be at least %d", name, minimum)
	}
	return nil
}

func validateMultiplier(name string, multiplier float64) error {
	if multiplier < 1 {
		return fmt.Errorf("invalid config: %s must be greater or equal to 1", name)
	}
	return nil
}
