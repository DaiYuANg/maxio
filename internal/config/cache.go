package config

import "time"

func (cfg Config) CacheTTLDuration() time.Duration {
	return parseDurationOr(cfg.CacheTTL, time.Minute)
}
