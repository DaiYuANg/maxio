// Package config loads and normalizes MaxIO runtime configuration.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/arcgolabs/configx"
	"github.com/lyonbrown4d/maxio/internal/model"
)

const defaultConfigPath = "./config.json"

type Config struct {
	HTTPAddress                  string           `json:"http_address"             koanf:"http_address"             validate:"required,min=1"`
	HTTPBodyLimit                int              `json:"http_body_limit"          koanf:"http_body_limit"`
	StorageAddress               string           `json:"storage_address"          koanf:"storage_address"`
	AdminToken                   string           `json:"admin_token"              koanf:"admin_token"`
	APIToken                     string           `json:"api_token"                koanf:"api_token"`
	EnableNativeObjectAPI        bool             `json:"enable_native_object_api" koanf:"enable_native_object_api"`
	MetadataBackend              string           `json:"metadata_backend"         koanf:"metadata_backend"         validate:"required,oneof=sqlite postgres mysql"`
	MetadataDSN                  string           `json:"metadata_dsn"             koanf:"metadata_dsn"`
	MetadataAutoMigrate          bool             `json:"metadata_auto_migrate"    koanf:"metadata_auto_migrate"`
	EnableS3Proxy                bool             `json:"enable_s3_proxy"          koanf:"enable_s3_proxy"`
	S3ProxyUpstreams             []model.Upstream `json:"s3_proxy_upstreams"       koanf:"s3_proxy_upstreams"`
	S3ProxyEntrypoint            string           `json:"s3_proxy_entrypoint"      koanf:"s3_proxy_entrypoint"`
	S3ProxyAdminAddress          string           `json:"s3_proxy_admin_address"   koanf:"s3_proxy_admin_address"`
	S3ProxyHealthInterval        string           `json:"s3_proxy_health_interval" koanf:"s3_proxy_health_interval"`
	S3ProxyHealthTimeout         string           `json:"s3_proxy_health_timeout"  koanf:"s3_proxy_health_timeout"`
	CacheBackend                 string           `json:"cache_backend"            koanf:"cache_backend"            validate:"required,oneof=none memory redis"`
	CacheTTL                     string           `json:"cache_ttl"                koanf:"cache_ttl"                validate:"required,min=1"`
	CacheMaxCost                 int              `json:"cache_max_cost"           koanf:"cache_max_cost"`
	CacheRedisAddress            string           `json:"cache_redis_address"      koanf:"cache_redis_address"`
	CacheRedisUsername           string           `json:"cache_redis_username"     koanf:"cache_redis_username"`
	CacheRedisPassword           string           `json:"cache_redis_password"     koanf:"cache_redis_password"`
	CacheRedisDB                 int              `json:"cache_redis_db"           koanf:"cache_redis_db"`
	CacheKeyPrefix               string           `json:"cache_key_prefix"         koanf:"cache_key_prefix"`
	DataDir                      string           `json:"data_dir"                 koanf:"data_dir"                 validate:"required,min=1"`
	LogLevel                     string           `json:"log_level"                koanf:"log_level"                validate:"required,oneof=debug info warn error"`
	NodeID                       uint64           `json:"node_id"                  koanf:"node_id"`
	GossipBindAddress            string           `json:"gossip_bind_address"      koanf:"gossip_bind_address"      validate:"required,min=1"`
	GossipAdvertiseAddress       string           `json:"gossip_advertise_address" koanf:"gossip_advertise_address"`
	GossipSeeds                  string           `json:"gossip_seeds"             koanf:"gossip_seeds"`
	PendingObjectTTL             string           `json:"pending_object_ttl"       koanf:"pending_object_ttl"       validate:"required,min=1"`
	RepairInterval               string           `json:"repair_interval"          koanf:"repair_interval"          validate:"required,min=1"`
	RepairOnStart                bool             `json:"repair_on_start"          koanf:"repair_on_start"`
	RepairMaxBatch               int              `json:"repair_max_batch"         koanf:"repair_max_batch"`
	RepairMaxRetries             int              `json:"repair_max_retries"       koanf:"repair_max_retries"`
	RepairRateLimit              int              `json:"repair_rate_limit"        koanf:"repair_rate_limit"`
	RepairRetryBackoff           string           `json:"repair_retry_backoff"     koanf:"repair_retry_backoff"     validate:"required,min=1"`
	RepairRetryMaxBackoff        string           `json:"repair_retry_max_backoff" koanf:"repair_retry_max_backoff"`
	RepairRetryBackoffMultiplier float64          `json:"repair_retry_multiplier"  koanf:"repair_retry_multiplier"`
	DedupeInterval               string           `json:"dedupe_interval"          koanf:"dedupe_interval"          validate:"required,min=1"`
	DedupeOnStart                bool             `json:"dedupe_on_start"          koanf:"dedupe_on_start"`
	DedupeMaxFixes               int              `json:"dedupe_max_fixes"         koanf:"dedupe_max_fixes"`
	IndexTimeout                 string           `json:"index_timeout"            koanf:"index_timeout"            validate:"required,min=1"`
	IndexRetryBackoff            string           `json:"index_retry_backoff"      koanf:"index_retry_backoff"      validate:"required,min=1"`
	IndexMaxRetries              int              `json:"index_max_retries"        koanf:"index_max_retries"`
	IndexQueueSize               int              `json:"index_queue_size"         koanf:"index_queue_size"`
	IndexRateLimit               int              `json:"index_rate_limit"         koanf:"index_rate_limit"`
}

func Load(opts ...configx.Option) (Config, error) {
	cfg := Default()
	options, err := loadOptions(cfg, opts...)
	if err != nil {
		return cfg, err
	}

	loaded, err := configx.LoadTErr[Config](options...)
	if err != nil {
		return cfg, fmt.Errorf("load config failed: %w", err)
	}
	return normalize(loaded)
}

func loadOptions(cfg Config, opts ...configx.Option) ([]configx.Option, error) {
	options := defaultLoadOptions(cfg)
	fileOptions, err := configFileOptions(defaultConfigPath)
	if err != nil {
		return nil, err
	}
	options = append(options, fileOptions...)
	options = append(options, opts...)
	return options, nil
}

func defaultLoadOptions(cfg Config) []configx.Option {
	return []configx.Option{
		configx.WithTypedDefaults(cfg),
		configx.WithDotenv(),
		configx.WithEnvPrefix("MAXIO"),
		configx.WithEnvSeparator("__"),
		configx.WithPriority(
			configx.SourceDotenv,
			configx.SourceFile,
			configx.SourceEnv,
			configx.SourceArgs,
		),
		configx.WithValidateLevel(configx.ValidateLevelStruct),
		configx.WithCommandLineFlags(),
		configx.WithLogger(slog.Default()),
		configx.WithWatchErrHandler(func(err error) {
			slog.Default().Error("config watch error", "error", err)
		}),
	}
}

func configFileOptions(path string) ([]configx.Option, error) {
	if _, statErr := os.Stat(path); statErr == nil {
		return []configx.Option{configx.WithFiles(path)}, nil
	} else if !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("check config path: %w", statErr)
	}
	return []configx.Option{}, nil
}

func normalize(cfg Config) (Config, error) {
	cfg = trim(cfg)
	if cfg.DataDir == "" {
		return cfg, errors.New("invalid config: data_dir is required")
	}
	cfg.DataDir = filepath.Clean(cfg.DataDir)

	if err := validateRequired(cfg); err != nil {
		return cfg, err
	}
	cfg = applyZeroDefaults(cfg)
	if err := validateDurations(cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func trim(cfg Config) Config {
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	cfg.HTTPAddress = strings.TrimSpace(cfg.HTTPAddress)
	cfg.MetadataBackend = strings.TrimSpace(strings.ToLower(cfg.MetadataBackend))
	cfg.MetadataDSN = strings.TrimSpace(cfg.MetadataDSN)
	cfg.StorageAddress = strings.TrimSpace(cfg.StorageAddress)
	cfg.AdminToken = strings.TrimSpace(cfg.AdminToken)
	cfg.APIToken = strings.TrimSpace(cfg.APIToken)
	cfg.CacheBackend = strings.ToLower(strings.TrimSpace(cfg.CacheBackend))
	cfg.CacheTTL = strings.TrimSpace(cfg.CacheTTL)
	cfg.CacheRedisAddress = strings.TrimSpace(cfg.CacheRedisAddress)
	cfg.CacheRedisUsername = strings.TrimSpace(cfg.CacheRedisUsername)
	cfg.CacheRedisPassword = strings.TrimSpace(cfg.CacheRedisPassword)
	cfg.CacheKeyPrefix = strings.TrimSpace(cfg.CacheKeyPrefix)
	cfg.LogLevel = strings.TrimSpace(cfg.LogLevel)
	cfg.S3ProxyEntrypoint = strings.TrimSpace(cfg.S3ProxyEntrypoint)
	cfg.S3ProxyAdminAddress = strings.TrimSpace(cfg.S3ProxyAdminAddress)
	cfg.S3ProxyHealthInterval = strings.TrimSpace(cfg.S3ProxyHealthInterval)
	cfg.S3ProxyHealthTimeout = strings.TrimSpace(cfg.S3ProxyHealthTimeout)
	cfg.GossipBindAddress = strings.TrimSpace(cfg.GossipBindAddress)
	cfg.GossipAdvertiseAddress = strings.TrimSpace(cfg.GossipAdvertiseAddress)
	cfg.GossipSeeds = strings.TrimSpace(cfg.GossipSeeds)
	cfg.PendingObjectTTL = strings.TrimSpace(cfg.PendingObjectTTL)
	cfg.RepairInterval = strings.TrimSpace(cfg.RepairInterval)
	cfg.RepairRetryBackoff = strings.TrimSpace(cfg.RepairRetryBackoff)
	cfg.RepairRetryMaxBackoff = strings.TrimSpace(cfg.RepairRetryMaxBackoff)
	cfg.DedupeInterval = strings.TrimSpace(cfg.DedupeInterval)
	cfg.IndexTimeout = strings.TrimSpace(cfg.IndexTimeout)
	cfg.IndexRetryBackoff = strings.TrimSpace(cfg.IndexRetryBackoff)
	return cfg
}

func validateRequired(cfg Config) error {
	if cfg.HTTPAddress == "" {
		return errors.New("invalid config: http_address is required")
	}
	if cfg.LogLevel == "" {
		return errors.New("invalid config: log_level is required")
	}
	return nil
}

func applyRepairZeroDefaults(cfg Config) Config {
	if cfg.RepairInterval == "" {
		cfg.RepairInterval = Default().RepairInterval
	}
	if cfg.RepairRetryBackoff == "" {
		cfg.RepairRetryBackoff = Default().RepairRetryBackoff
	}
	if cfg.RepairRetryMaxBackoff == "" {
		cfg.RepairRetryMaxBackoff = Default().RepairRetryMaxBackoff
	}
	if cfg.RepairMaxBatch <= 0 {
		cfg.RepairMaxBatch = Default().RepairMaxBatch
	}
	if cfg.RepairRetryBackoffMultiplier <= 0 {
		cfg.RepairRetryBackoffMultiplier = Default().RepairRetryBackoffMultiplier
	}
	return cfg
}

func applyDedupeZeroDefaults(cfg Config) Config {
	if cfg.DedupeInterval == "" {
		cfg.DedupeInterval = Default().DedupeInterval
	}
	if cfg.DedupeMaxFixes <= 0 {
		cfg.DedupeMaxFixes = Default().DedupeMaxFixes
	}
	return cfg
}
