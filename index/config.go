package index

import (
	"path/filepath"
	"strings"
)

type Config struct {
	DataDir string
}

func (cfg Config) normalized() Config {
	cfg.DataDir = strings.TrimSpace(cfg.DataDir)
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	cfg.DataDir = filepath.Clean(cfg.DataDir)
	return cfg
}
