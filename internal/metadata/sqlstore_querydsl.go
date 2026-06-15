package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	collectionlist "github.com/arcgolabs/collectionx/list"
	"github.com/arcgolabs/dbx/querydsl"
)

type sqlRowScanner interface {
	Scan(dest ...any) error
}

func (s *SQLMetadata) queryBuilderContext(ctx context.Context, query querydsl.Builder) (*sql.Rows, error) {
	if s == nil || s.dbxDB == nil {
		return nil, errors.New("metadata dbx session is nil")
	}

	bound, err := query.Build(s.dbxDB.Dialect())
	if err != nil {
		return nil, fmt.Errorf("build metadata query: %w", err)
	}
	rows, err := s.dbxDB.QueryBoundContext(ensureContext(ctx), bound)
	if err != nil {
		return nil, fmt.Errorf("query metadata rows: %w", err)
	}
	return rows, nil
}

func (s *SQLMetadata) queryRowBuilderContext(ctx context.Context, query querydsl.Builder) (sqlRowScanner, error) {
	if s == nil || s.dbxDB == nil {
		return nil, errors.New("metadata dbx session is nil")
	}

	bound, err := query.Build(s.dbxDB.Dialect())
	if err != nil {
		return nil, fmt.Errorf("build metadata query: %w", err)
	}
	return s.dbxDB.QueryRowContext(ensureContext(ctx), bound.SQL, boundArgs(bound.Args)...), nil
}

func (s *SQLMetadata) execBuilderContext(ctx context.Context, query querydsl.Builder) (sql.Result, error) {
	if s == nil || s.dbxDB == nil {
		return nil, errors.New("metadata dbx session is nil")
	}

	bound, err := query.Build(s.dbxDB.Dialect())
	if err != nil {
		return nil, fmt.Errorf("build metadata mutation: %w", err)
	}
	result, err := s.dbxDB.ExecBoundContext(ensureContext(ctx), bound)
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
	rows, err := tx.QueryContext(ensureContext(ctx), bound.SQL, boundArgs(bound.Args)...)
	if err != nil {
		return nil, fmt.Errorf("query metadata tx rows: %w", err)
	}
	return rows, nil
}

func (s *SQLMetadata) txQueryRowBuilderContext(ctx context.Context, tx *sql.Tx, query querydsl.Builder) (sqlRowScanner, error) {
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
	return tx.QueryRowContext(ensureContext(ctx), bound.SQL, boundArgs(bound.Args)...), nil
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
	if _, err := tx.ExecContext(ensureContext(ctx), bound.SQL, boundArgs(bound.Args)...); err != nil {
		return fmt.Errorf("exec metadata tx query: %w", err)
	}
	return nil
}

func boundArgs(queryArgs *collectionlist.List[any]) []any {
	if queryArgs == nil {
		return nil
	}
	var result []any
	queryArgs.ViewValues(func(values []any) {
		result = values
	})
	return result
}
