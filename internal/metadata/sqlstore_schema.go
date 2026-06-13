package metadata

import (
	"context"
	"embed"
	"errors"
	"fmt"

	dbxmigrate "github.com/arcgolabs/dbx/migrate"
)

const (
	metadataMigrationDir          = "migrations"
	metadataMigrationHistoryTable = "metadata_schema_history"
)

//go:embed migrations/*.sql
var metadataMigrationFS embed.FS

func (s *SQLMetadata) createSchema(ctx context.Context) error {
	if s == nil || s.db == nil || s.dbxDB == nil {
		return errors.New("metadata db session is nil")
	}

	selector, err := dbxmigrate.DialectFromDialect(s.dbxDB.Dialect())
	if err != nil {
		return fmt.Errorf("resolve metadata migration dialect: %w", err)
	}

	runner := dbxmigrate.NewRunner(
		s.db,
		s.dbxDB.Dialect(),
		dbxmigrate.RunnerOptions{
			HistoryTable: metadataMigrationHistoryTable,
			ValidateHash: true,
		},
	)
	source := dbxmigrate.FileSource{
		FS:       metadataMigrationFS,
		Dir:      metadataMigrationDir,
		Database: selector,
	}

	report, err := dbxmigrate.SQL(runner).ForDialect(selector).Up(ensureContext(ctx), source)
	if err != nil {
		return fmt.Errorf("apply metadata migrations: %w", err)
	}
	if s.logger != nil {
		s.logger.Info("metadata migrations applied", "dialect", selector.String(), "count", report.Applied.Len())
	}
	return nil
}
