package metadata

import (
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/lyonbrown4d/maxio/internal/config"
)

const (
	metadataBackendSQLite   = "sqlite"
	metadataBackendPostgres = "postgres"
	metadataBackendMySQL    = "mysql"
)

func NewMetadataStore(cfg config.Config, logger *slog.Logger) (MetadataStore, error) {
	if logger == nil {
		logger = slog.Default()
	}

	backend := strings.TrimSpace(strings.ToLower(cfg.MetadataBackend))
	if backend == "" {
		backend = metadataBackendSQLite
	}

	switch backend {
	case metadataBackendSQLite:
		dsn := strings.TrimSpace(cfg.MetadataDSN)
		if dsn == "" {
			dsn = filepath.Join(cfg.DataDir, "metadata.db")
		}
		return NewSQLiteMetadata(dsn, logger)
	case metadataBackendPostgres:
		return NewPostgresMetadata(strings.TrimSpace(cfg.MetadataDSN), logger, cfg.MetadataAutoMigrate)
	case metadataBackendMySQL:
		return NewMySQLMetadata(strings.TrimSpace(cfg.MetadataDSN), logger, cfg.MetadataAutoMigrate)
	default:
		return nil, fmt.Errorf("unsupported metadata backend: %s", cfg.MetadataBackend)
	}
}
