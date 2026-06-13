package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
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

	store := &SQLMetadata{
		db:           db,
		logger:       logger,
		queryDialect: metadataSQLDialectPostgres,
	}
	if err := store.ping(context.Background()); err != nil {
		return nil, fmt.Errorf("connect postgres metadata database: %w", closePostgresOnInitError(db, logger, err))
	}
	if migrate {
		if err := store.createSchema(context.Background()); err != nil {
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
