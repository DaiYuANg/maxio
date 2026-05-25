package config

import "time"

func (cfg Config) CacheTTLDuration() time.Duration {
	duration, err := time.ParseDuration(cfg.CacheTTL)
	if err != nil {
		return time.Minute
	}
	return duration
}
