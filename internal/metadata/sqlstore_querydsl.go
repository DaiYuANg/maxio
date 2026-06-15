package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/arcgolabs/dbx"
	"github.com/arcgolabs/dbx/querydsl"
)

func (s *SQLMetadata) execBuilderContext(ctx context.Context, query querydsl.Builder) (sql.Result, error) {
	if s == nil || s.dbxDB == nil {
		return nil, errors.New("metadata dbx session is nil")
	}

	result, err := dbx.Exec(ensureContext(ctx), s.dbxDB, query)
	if err != nil {
		return nil, fmt.Errorf("exec metadata query: %w", err)
	}
	return result, nil
}

func (s *SQLMetadata) txQueryBuilderContext(ctx context.Context, tx *sql.Tx, query querydsl.Builder) (*sql.Rows, error) {
	if s == nil || s.dbxDB == nil {
		return nil, errors.New("metadata dbx session is nil")
	}
	if tx == nil {
		return nil, errors.New("metadata tx is nil")
	}

	bound, err := query.Build(s.dbxDB.Dialect())
	if err != nil {
		return nil, fmt.Errorf("build metadata tx query: %w", err)
	}
	rows, err := s.dbxDB.WithTx(tx).QueryBoundContext(ensureContext(ctx), bound)
	if err != nil {
		return nil, fmt.Errorf("query metadata tx rows: %w", err)
	}
	return rows, nil
}

func (s *SQLMetadata) txExecBuilderContext(ctx context.Context, tx *sql.Tx, query querydsl.Builder) error {
	if s == nil || s.dbxDB == nil {
		return errors.New("metadata dbx session is nil")
	}
	if tx == nil {
		return errors.New("metadata tx is nil")
	}

	bound, err := query.Build(s.dbxDB.Dialect())
	if err != nil {
		return fmt.Errorf("build metadata tx mutation: %w", err)
	}
	if _, err := s.dbxDB.WithTx(tx).ExecBoundContext(ensureContext(ctx), bound); err != nil {
		return fmt.Errorf("exec metadata tx query: %w", err)
	}
	return nil
}
