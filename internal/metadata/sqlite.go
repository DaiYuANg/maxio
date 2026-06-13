package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/arcgolabs/dbx"
	sqlitedialect "github.com/arcgolabs/dbx/dialect/sqlite"
	// Register the pure-Go SQLite database/sql driver.
	_ "modernc.org/sqlite"
)

type SQLiteMetadata struct {
	db     *sql.DB
	dbxDB  *dbx.DB
	logger *slog.Logger
}

func NewSQLiteMetadata(path string, logger *slog.Logger) (*SQLiteMetadata, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("metadata database path is required")
	}

	databaseDir := filepath.Dir(path)
	if err := os.MkdirAll(databaseDir, 0o750); err != nil {
		return nil, fmt.Errorf("create metadata directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	session, err := dbx.NewWithOptions(db, sqlitedialect.New(), dbx.WithLogger(logger))
	if err != nil {
		return nil, fmt.Errorf("initialize dbx metadata session: %w", closeSQLiteOnInitError(db, logger, err))
	}

	metadata := &SQLiteMetadata{
		db:     db,
		dbxDB:  session,
		logger: logger,
	}

	if err := metadata.applyPragmas(context.Background()); err != nil {
		return nil, fmt.Errorf("configure sqlite: %w", closeSQLiteOnInitError(db, logger, err))
	}
	if err := metadata.createSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("initialize sqlite schema: %w", closeSQLiteOnInitError(db, logger, err))
	}

	return metadata, nil
}

func (s *SQLiteMetadata) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	if s.dbxDB != nil {
		if err := s.dbxDB.Close(); err != nil {
			return fmt.Errorf("close dbx metadata session: %w", err)
		}
		return nil
	}
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close sqlite database: %w", err)
	}
	return nil
}

func (s *SQLiteMetadata) applyPragmas(ctx context.Context) error {
	ctx = ensureContext(ctx)
	stmts := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, stmt := range stmts {
		if _, err := s.execContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec sqlite pragma: %w", err)
		}
	}
	return nil
}

func (s *SQLiteMetadata) createSchema(ctx context.Context) error {
	ctx = ensureContext(ctx)
	for _, stmt := range sqliteSchemaStatements {
		if _, err := s.execContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec schema statement: %w", err)
		}
	}
	return nil
}

func (s *SQLiteMetadata) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if s == nil {
		return nil, errors.New("metadata db session is nil")
	}
	session := s.db
	if s.dbxDB != nil {
		return s.dbxDB.ExecContext(ctx, query, args...)
	}
	return session.ExecContext(ctx, query, args...)
}

func (s *SQLiteMetadata) withTx(ctx context.Context, op string, fn func(*sql.Tx) error) error {
	ctx = ensureContext(ctx)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin %s transaction: %w", op, err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) && s.logger != nil {
			s.logger.Error("rollback sqlite transaction", "op", op, "error", rollbackErr)
		}
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit %s transaction: %w", op, err)
	}
	committed = true
	return nil
}

func (s *SQLiteMetadata) closeRows(rows *sql.Rows, label string) {
	if closeErr := rows.Close(); closeErr != nil && s.logger != nil {
		s.logger.Error("close sqlite rows", "rows", label, "error", closeErr)
	}
}

func closeSQLiteOnInitError(db *sql.DB, logger *slog.Logger, cause error) error {
	if closeErr := db.Close(); closeErr != nil {
		if logger != nil {
			logger.Error("close sqlite database after init failure", "error", closeErr)
		}
		return errors.Join(cause, fmt.Errorf("close sqlite database: %w", closeErr))
	}
	return cause
}
