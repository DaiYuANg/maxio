package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/arcgolabs/dbx"
	sqlitedialect "github.com/arcgolabs/dbx/dialect/sqlite"
	// Register the pure-Go SQLite database/sql driver.
	_ "modernc.org/sqlite"
)

type metadataQueryDialect string

const (
	metadataQueryDialectSQLite   metadataQueryDialect = "sqlite"
	metadataQueryDialectPostgres metadataQueryDialect = "postgres"
)

type SQLiteMetadata struct {
	db           *sql.DB
	dbxDB        *dbx.DB
	logger       *slog.Logger
	queryDialect metadataQueryDialect
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
		db:           db,
		dbxDB:        session,
		logger:       logger,
		queryDialect: metadataQueryDialectSQLite,
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
	query = s.normalizeQuery(query)
	session := s.db
	if s.dbxDB != nil {
		result, err := s.dbxDB.ExecContext(ctx, query, args...)
		if err != nil {
			return nil, fmt.Errorf("exec metadata query: %w", err)
		}
		return result, nil
	}
	result, err := session.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("exec metadata query: %w", err)
	}
	return result, nil
}

func (s *SQLiteMetadata) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	query = s.normalizeQuery(query)
	rows, err := s.db.QueryContext(ensureContext(ctx), query, args...)
	if err != nil {
		return nil, fmt.Errorf("query metadata rows: %w", err)
	}
	return rows, nil
}

func (s *SQLiteMetadata) queryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	query = s.normalizeQuery(query)
	return s.db.QueryRowContext(ensureContext(ctx), query, args...)
}

func (s *SQLiteMetadata) txQueryContext(ctx context.Context, tx *sql.Tx, query string, args ...any) (*sql.Rows, error) {
	query = s.normalizeQuery(query)
	rows, err := tx.QueryContext(ensureContext(ctx), query, args...)
	if err != nil {
		return nil, fmt.Errorf("query metadata tx rows: %w", err)
	}
	return rows, nil
}

func (s *SQLiteMetadata) txQueryRowContext(ctx context.Context, tx *sql.Tx, query string, args ...any) *sql.Row {
	query = s.normalizeQuery(query)
	return tx.QueryRowContext(ensureContext(ctx), query, args...)
}

func (s *SQLiteMetadata) txExecContext(ctx context.Context, tx *sql.Tx, query string, args ...any) error {
	query = s.normalizeQuery(query)
	_, err := tx.ExecContext(ensureContext(ctx), query, args...)
	if err != nil {
		return fmt.Errorf("exec metadata tx query: %w", err)
	}
	return nil
}

func (s *SQLiteMetadata) normalizeQuery(query string) string {
	if s == nil {
		return query
	}
	if s.queryDialect == metadataQueryDialectPostgres {
		return rewriteSQLitePlaceholdersToPostgres(query)
	}
	return query
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

func closeSQLiteOnInitError(db *sql.DB, logger *slog.Logger, cause error) error {
	if closeErr := db.Close(); closeErr != nil {
		if logger != nil {
			logger.Error("close sqlite database after init failure", "error", closeErr)
		}
		return errors.Join(cause, fmt.Errorf("close sqlite database: %w", closeErr))
	}
	return cause
}

type placeholderRewriteState struct {
	builder      strings.Builder
	inSingle     bool
	inDouble     bool
	position     int
	prevWasSlash bool
}

func rewriteSQLitePlaceholdersToPostgres(query string) string {
	var state placeholderRewriteState
	for i := range len(query) {
		state.write(query[i])
	}
	return state.builder.String()
}

func (s *placeholderRewriteState) write(ch byte) {
	if s.writeEscaped(ch) {
		return
	}
	if s.writeQuote(ch) {
		return
	}
	if s.writePlaceholder(ch) {
		return
	}
	s.writeByte(ch)
}

func (s *placeholderRewriteState) writeEscaped(ch byte) bool {
	if s.prevWasSlash {
		s.prevWasSlash = false
		s.writeByte(ch)
		return true
	}
	if ch != '\\' {
		return false
	}
	s.prevWasSlash = true
	s.writeByte(ch)
	return true
}

func (s *placeholderRewriteState) writeQuote(ch byte) bool {
	if ch == '\'' && !s.inDouble {
		s.inSingle = !s.inSingle
		s.writeByte(ch)
		return true
	}
	if ch != '"' || s.inSingle {
		return false
	}
	s.inDouble = !s.inDouble
	s.writeByte(ch)
	return true
}

func (s *placeholderRewriteState) writePlaceholder(ch byte) bool {
	if ch != '?' || s.inSingle || s.inDouble {
		return false
	}
	s.position++
	s.writeString("$")
	s.writeString(strconv.Itoa(s.position))
	return true
}

func (s *placeholderRewriteState) writeByte(ch byte) {
	if err := s.builder.WriteByte(ch); err != nil {
		panic(fmt.Errorf("write query byte: %w", err))
	}
}

func (s *placeholderRewriteState) writeString(value string) {
	if _, err := s.builder.WriteString(value); err != nil {
		panic(fmt.Errorf("write query string: %w", err))
	}
}
