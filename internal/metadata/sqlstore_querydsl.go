package metadata

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
	return s.dbxDB.QueryRowContext(ensureContext(ctx), bound.SQL, bound.Args.Values()...), nil
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
