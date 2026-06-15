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
	sqltmpl "github.com/arcgolabs/dbx/sqltmpl"
	// Register the pure-Go SQLite database/sql driver.
	_ "modernc.org/sqlite"
)

type SQLMetadata struct {
	db           *sql.DB
	dbxDB        *dbx.DB
	sqlTemplates *sqltmpl.Registry
	repos        metadataSQLRepositories
	logger       *slog.Logger
}

type SQLiteMetadata = SQLMetadata

func NewSQLiteMetadata(path string, logger *slog.Logger, migrateSchema ...bool) (*SQLMetadata, error) {
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
		return nil, fmt.Errorf("initialize dbx metadata session: %w", closeSQLMetadataOnInitError(db, logger, err))
	}

	metadata := &SQLMetadata{
		db:           db,
		dbxDB:        session,
		sqlTemplates: newMetadataSQLTemplateRegistry(session),
		repos:        newMetadataSQLRepositories(session),
		logger:       logger,
	}

	if err := metadata.applyPragmas(context.Background()); err != nil {
		return nil, fmt.Errorf("configure sqlite: %w", closeSQLMetadataOnInitError(db, logger, err))
	}
	if shouldMigrateSchema(migrateSchema...) {
		if err := metadata.createSchema(context.Background()); err != nil {
			return nil, fmt.Errorf("initialize sqlite schema: %w", closeSQLMetadataOnInitError(db, logger, err))
		}
	}

	return metadata, nil
}

func shouldMigrateSchema(values ...bool) bool {
	if len(values) == 0 {
		return true
	}
	return values[0]
}

func newSQLMetadata(
	db *sql.DB,
	session *dbx.DB,
	logger *slog.Logger,
	migrateSchema bool,
) (*SQLMetadata, error) {
	metadata := &SQLMetadata{
		db:           db,
		dbxDB:        session,
		sqlTemplates: newMetadataSQLTemplateRegistry(session),
		repos:        newMetadataSQLRepositories(session),
		logger:       logger,
	}
	if migrateSchema {
		if err := metadata.createSchema(context.Background()); err != nil {
			return nil, fmt.Errorf("initialize metadata schema: %w", err)
		}
	}
	return metadata, nil
}

func NewSQLMetadata(path string, logger *slog.Logger) (*SQLMetadata, error) {
	return NewSQLiteMetadata(path, logger)
}

func (s *SQLMetadata) Close() error {
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

func (s *SQLMetadata) applyPragmas(ctx context.Context) error {
	ctx = ensureContext(ctx)
	stmts := []string{
		metadataSQLPragmaJournalMode,
		metadataSQLPragmaSynchronous,
		metadataSQLPragmaForeignKeys,
		metadataSQLPragmaBusyTimeout,
	}
	for _, stmt := range stmts {
		if _, err := s.execSQLTemplateContext(ctx, stmt, nil); err != nil {
			return fmt.Errorf("exec sqlite pragma: %w", err)
		}
	}
	return nil
}

func (s *SQLMetadata) withTx(ctx context.Context, op string, fn func(*sql.Tx) error) error {
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
			s.logger.Error("rollback sql metadata transaction", "op", op, "error", rollbackErr)
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

func closeSQLMetadataOnInitError(db *sql.DB, logger *slog.Logger, cause error) error {
	if closeErr := db.Close(); closeErr != nil {
		if logger != nil {
			logger.Error("close sql metadata database after init failure", "error", closeErr)
		}
		return errors.Join(cause, fmt.Errorf("close sql metadata database: %w", closeErr))
	}
	return cause
}
