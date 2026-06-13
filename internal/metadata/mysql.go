package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/go-sql-driver/mysql"
)

const metadataSQLDriverMySQL = "mysql"

func NewMySQLMetadata(dsn string, logger *slog.Logger, migrate bool) (*SQLMetadata, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("metadata mysql DSN is required")
	}

	db, err := sql.Open(metadataSQLDriverMySQL, strings.TrimSpace(dsn))
	if err != nil {
		return nil, fmt.Errorf("open mysql metadata database: %w", err)
	}

	store := &SQLMetadata{
		db:           db,
		logger:       logger,
		queryDialect: metadataSQLDialectMySQL,
	}
	if err := store.ping(context.Background()); err != nil {
		return nil, fmt.Errorf("connect mysql metadata database: %w", closeMySQLOnInitError(db, logger, err))
	}
	if migrate {
		if err := store.createSchema(context.Background()); err != nil {
			return nil, fmt.Errorf("initialize mysql schema: %w", closeMySQLOnInitError(db, logger, err))
		}
	}

	return store, nil
}

func closeMySQLOnInitError(db *sql.DB, logger *slog.Logger, cause error) error {
	if closeErr := db.Close(); closeErr != nil {
		if logger != nil {
			logger.Error("close mysql database after init failure", "error", closeErr)
		}
		return fmt.Errorf("%w: %w", cause, closeErr)
	}
	return cause
}
