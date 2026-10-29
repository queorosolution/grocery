// Package migrations embeds the SQL migrations so the migrate command, CI and
// the test database all apply exactly the same files.
package migrations

import (
	"database/sql"
	"embed"

	"github.com/pressly/goose/v3"
)

//go:embed *.sql
var files embed.FS

// Provider returns a goose provider for the embedded migrations. Run it as the
// database owner, never as app_rw (docs/05 section 5).
func Provider(db *sql.DB) (*goose.Provider, error) {
	return goose.NewProvider(goose.DialectPostgres, db, files)
}
