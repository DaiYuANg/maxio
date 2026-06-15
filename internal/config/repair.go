package config

import "time"

func (cfg Config) RepairIntervalDuration() time.Duration {
	return parseDurationOr(cfg.RepairInterval, 10*time.Minute)
}

func (cfg Config) RepairRetryBackoffDuration() time.Duration {
	return parseDurationOr(cfg.RepairRetryBackoff, time.Second)
}

func (cfg Config) RepairRetryMaxBackoffDuration() time.Duration {
	return parseDurationOr(cfg.RepairRetryMaxBackoff, 10*time.Second)
}

func (cfg Config) RepairRetryMultiplier() float64 {
	if cfg.RepairRetryBackoffMultiplier <= 1 {
		return 1
	}
	return cfg.RepairRetryBackoffMultiplier
}

func (cfg Config) DedupeIntervalDuration() time.Duration {
	return parseDurationOr(cfg.DedupeInterval, 30*time.Minute)
}

func (cfg Config) IndexTimeoutDuration() time.Duration {
	return parseDurationOr(cfg.IndexTimeout, 30*time.Second)
}

func (cfg Config) IndexRetryBackoffDuration() time.Duration {
	return parseDurationOr(cfg.IndexRetryBackoff, time.Second)
}
