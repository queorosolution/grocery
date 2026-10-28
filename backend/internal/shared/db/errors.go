package db

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	codeUnique     = "23505"
	codeForeignKey = "23503"
	codeCheck      = "23514"
)

// IgnoreNoRows turns "no row" into nil, for lookups that report found separately.
func IgnoreNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

// IsUniqueViolation reports a unique index or constraint clash.
func IsUniqueViolation(err error) bool { return hasCode(err, codeUnique) }

// IsForeignKeyViolation reports a reference to a row that does not exist.
func IsForeignKeyViolation(err error) bool { return hasCode(err, codeForeignKey) }

// IsCheckViolation reports a failed CHECK constraint.
func IsCheckViolation(err error) bool { return hasCode(err, codeCheck) }

func hasCode(err error, code string) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == code
}
