package metadata

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/sqlstmt"
	sqltmpl "github.com/arcgolabs/dbx/sqltmpl"
)

//go:embed sql
var metadataSQLTemplates embed.FS

const (
	metadataSQLBlobIncreaseRefCount = "sql/blob/increase_ref_count.sql"
	metadataSQLBlobDecreaseRefCount = "sql/blob/decrease_ref_count.sql"

	metadataSQLPragmaJournalMode = "sql/sqlite/pragma_journal_mode.sql"
	metadataSQLPragmaSynchronous = "sql/sqlite/pragma_synchronous.sql"
	metadataSQLPragmaForeignKeys = "sql/sqlite/pragma_foreign_keys.sql"
	metadataSQLPragmaBusyTimeout = "sql/sqlite/pragma_busy_timeout.sql"
)

type metadataBlobRefHashParams struct {
	Hash string
}

func newMetadataSQLTemplateRegistry(session *dbx.DB) *sqltmpl.Registry {
	if session == nil {
		return nil
	}
	return sqltmpl.NewRegistry(metadataSQLTemplates, session.Dialect())
}

func (s *SQLMetadata) execSQLTemplateContext(ctx context.Context, name string, params any) (sql.Result, error) {
	if s == nil || s.dbxDB == nil {
		return nil, errors.New("metadata dbx session is nil")
	}
	bound, err := s.bindSQLTemplate(name, params)
	if err != nil {
		return nil, err
	}
	result, err := s.dbxDB.ExecBoundContext(ensureContext(ctx), bound)
	if err != nil {
		return nil, fmt.Errorf("exec metadata sql template %q: %w", name, err)
	}
	return result, nil
}

func (s *SQLMetadata) txExecSQLTemplateContext(ctx context.Context, tx *sql.Tx, name string, params any) error {
	if tx == nil {
		return errors.New("metadata tx is nil")
	}
	bound, err := s.bindSQLTemplate(name, params)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ensureContext(ctx), bound.SQL, boundArgs(bound.Args)...); err != nil {
		return fmt.Errorf("exec metadata tx sql template %q: %w", name, err)
	}
	return nil
}

func (s *SQLMetadata) bindSQLTemplate(name string, params any) (sqlstmt.Bound, error) {
	if s == nil || s.sqlTemplates == nil {
		return sqlstmt.Bound{}, errors.New("metadata sql template registry is nil")
	}
	template, err := s.sqlTemplates.Statement(name)
	if err != nil {
		return sqlstmt.Bound{}, fmt.Errorf("load metadata sql template %q: %w", name, err)
	}
	bound, err := template.Bind(params)
	if err != nil {
		return sqlstmt.Bound{}, fmt.Errorf("bind metadata sql template %q: %w", name, err)
	}
	return bound, nil
}
