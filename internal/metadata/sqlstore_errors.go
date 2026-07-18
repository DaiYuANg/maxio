package metadata

import (
	"errors"
	"strings"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const (
	mysqlDuplicateEntryCode    uint16 = 1062
	postgresUniqueViolationSQL        = "23505"
)

func isSQLConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return isSQLiteConstraintError(err) ||
		isMySQLConstraintError(err) ||
		isPostgresConstraintError(err) ||
		isFallbackSQLConstraintError(err)
}

func isSQLiteConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code()
	return code == sqlite3.SQLITE_CONSTRAINT || code&0xff == sqlite3.SQLITE_CONSTRAINT
}

func isMySQLConstraintError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	return mysqlErr.Number == mysqlDuplicateEntryCode
}

func isPostgresConstraintError(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}
	return pgErr.Code == postgresUniqueViolationSQL
}

func isFallbackSQLConstraintError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint failed") ||
		strings.Contains(message, "duplicate key value violates unique constraint") ||
		strings.Contains(message, "duplicate entry") ||
		strings.Contains(message, "error 1062") ||
		strings.Contains(message, "constraint failed")
}
