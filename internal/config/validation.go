package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/samber/mo"
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
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	if err := validateDurationConfigs(durationConfigs(cfg)); err != nil {
		return err
	}
	if err := validateIntegerConfigs(integerConfigs(cfg)); err != nil {
		return err
	}
	if err := validateMultiplier("repair_retry_multiplier", cfg.RepairRetryBackoffMultiplier); err != nil {
		return err
	}
	return validateIndexConfig(cfg)
}

func durationConfigs(cfg Config) []durationConfig {
	configs := []durationConfig{
		{name: "pending_object_ttl", value: cfg.PendingObjectTTL},
		{name: "repair_interval", value: cfg.RepairInterval},
		{name: "repair_retry_backoff", value: cfg.RepairRetryBackoff},
		{name: "repair_retry_max_backoff", value: cfg.RepairRetryMaxBackoff},
		{name: "dedupe_interval", value: cfg.DedupeInterval},
		{name: "index_timeout", value: cfg.IndexTimeout},
		{name: "index_retry_backoff", value: cfg.IndexRetryBackoff},
		{name: "cache_ttl", value: cfg.CacheTTL},
	}
	if cfg.EnableS3Proxy {
		configs = append(configs,
			durationConfig{name: "s3_proxy_health_interval", value: cfg.S3ProxyHealthInterval},
			durationConfig{name: "s3_proxy_health_timeout", value: cfg.S3ProxyHealthTimeout},
		)
	}
	return configs
}

func validateMetadataConfig(cfg Config) error {
	switch cfg.MetadataBackend {
	case "postgres", "mysql":
		if strings.TrimSpace(cfg.MetadataDSN) == "" {
			return fmt.Errorf("invalid config: metadata_dsn is required when metadata_backend is %s", cfg.MetadataBackend)
		}
	}
	return nil
}

func validateDurationConfigs(configs []durationConfig) error {
	for _, cfgValue := range configs {
		if err := validateDuration(cfgValue.name, cfgValue.value); err != nil {
			return err
		}
	}
	return nil
}

func integerConfigs(cfg Config) []intConfig {
	return []intConfig{
		{name: "repair_max_retries", value: cfg.RepairMaxRetries, minimum: 0},
		{name: "repair_rate_limit", value: cfg.RepairRateLimit, minimum: 0},
		{name: "dedupe_max_fixes", value: cfg.DedupeMaxFixes, minimum: 0},
		{name: "index_max_retries", value: cfg.IndexMaxRetries, minimum: 0},
		{name: "index_queue_size", value: cfg.IndexQueueSize, minimum: 0},
		{name: "index_rate_limit", value: cfg.IndexRateLimit, minimum: 0},
		{name: "cache_max_cost", value: cfg.CacheMaxCost, minimum: 0},
		{name: "cache_redis_db", value: cfg.CacheRedisDB, minimum: 0},
	}
}

func validateIntegerConfigs(configs []intConfig) error {
	for _, cfgValue := range configs {
		if err := validateNonNegativeInt(cfgValue.name, cfgValue.value, cfgValue.minimum); err != nil {
			return err
		}
	}
	return nil
}

func validateDuration(name, value string) error {
	if _, err := parseDuration(value); err != nil {
		return fmt.Errorf("invalid config: %s: %w", name, err)
	}
	return nil
}

func parseDuration(value string) (time.Duration, error) {
	result := mo.Try(func() (time.Duration, error) {
		return time.ParseDuration(value)
	})
	if result.IsError() {
		return 0, fmt.Errorf("parse duration %q: %w", value, result.Error())
	}
	return result.OrElse(0), nil
}

func parseDurationOr(value string, fallback time.Duration) time.Duration {
	duration, err := parseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
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
