package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/arcgolabs/dbx"
	postgresdialect "github.com/arcgolabs/dbx/dialect/postgres"
	_ "github.com/jackc/pgx/v5/stdlib" // Register pgx driver for database/sql.
)

func NewPostgresMetadata(dsn string, logger *slog.Logger, migrate bool) (*SQLMetadata, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("metadata postgres DSN is required")
	}

	db, err := sql.Open("pgx", strings.TrimSpace(dsn))
	if err != nil {
		return nil, fmt.Errorf("open postgres metadata database: %w", err)
	}

	session, err := dbx.NewWithOptions(db, postgresdialect.New(), dbx.WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("initialize dbx postgres metadata session: %w", closePostgresOnInitError(db, logger, err))
	}

	store := &SQLMetadata{db: db, dbxDB: session, sqlTemplates: newMetadataSQLTemplateRegistry(session), logger: logger}
	if pingErr := store.ping(context.Background()); pingErr != nil {
		return nil, fmt.Errorf("connect postgres metadata database: %w", closePostgresOnInitError(db, logger, pingErr))
	}
	if migrate {
		store, err = newSQLMetadata(db, session, logger, true)
		if err != nil {
			return nil, fmt.Errorf("initialize postgres schema: %w", closePostgresOnInitError(db, logger, err))
		}
	}

	logger.Info("metadata backend selected", "backend", "postgres")
	return store, nil
}

func (s *SQLMetadata) ping(ctx context.Context) error {
	if s == nil || s.db == nil {
		return errors.New("metadata db session is nil")
	}
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping metadata database: %w", err)
	}
	return nil
}

func closePostgresOnInitError(db *sql.DB, logger *slog.Logger, cause error) error {
	if closeErr := db.Close(); closeErr != nil {
		if logger != nil {
			logger.Error("close postgres database after init failure", "error", closeErr)
		}
		return fmt.Errorf("%w: %w", cause, closeErr)
	}
	return cause
}
