package source

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Limits (FR-SYNC-13).
const (
	connectTimeout   = 10 * time.Second
	statementTimeout = 20 * time.Second
	maxTables        = 1000
	maxColumns       = 500
)

// Opener opens a connection to a shop database.
type Opener interface {
	Open(ctx context.Context, connString string) (Conn, error)
}

// Conn is one open, read-only connection.
type Conn interface {
	Test(ctx context.Context) (TestInfo, error)
	Tables(ctx context.Context) ([]Table, error)
	Columns(ctx context.Context, t TableName) ([]Column, error)
	// Read runs a SELECT built from the spec and calls fn for each row.
	Read(ctx context.Context, spec ReadSpec, fn func(Row) error) error
	Close()
}

type Table struct {
	TableName
	Kind string `json:"kind"` // "TABLE" or "VIEW"
}

type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Row holds the selected columns as text, nil for NULL, in the order of ReadSpec.Columns.
type Row []*string

type TestInfo struct {
	ServerVersion string
	TLS           bool
	CanWrite      bool // the database user has write rights somewhere
}

// OpenError says how far a connection got. It carries no host, user or password.
type OpenError struct {
	Stage string // "string", "guard", "connect"
	Code  string // the SQLSTATE of a server error, if any
}

func (e *OpenError) Error() string { return "source: open failed at " + e.Stage + " " + e.Code }

// Options configure Postgres. The zero value is the production setting: only
// public hosts on port 5432, TLS with a verified certificate.
type Options struct {
	Policy Policy
	// PlaintextForTest connects without TLS, for a local test database. Never set in production.
	PlaintextForTest bool
}

// Postgres opens real connections, through the policy.
type Postgres struct{ opts Options }

func NewPostgres(o Options) *Postgres { return &Postgres{opts: o} }

// Open parses the string, checks the host and connects to the checked address.
// The driver only ever sees a rebuilt URL, so nothing else from the string is used.
func (p *Postgres) Open(ctx context.Context, connString string) (Conn, error) {
	t, err := ParseConnString(connString)
	if err != nil {
		return nil, &OpenError{Stage: "string"}
	}
	addr, err := p.opts.Policy.Resolve(ctx, t.Host, t.Port)
	if err != nil {
		return nil, &OpenError{Stage: "guard"}
	}
	cfg, err := pgx.ParseConfig(t.url(addr.String()))
	if err != nil {
		return nil, &OpenError{Stage: "string"}
	}
	// ParseConfig also reads PG* environment variables and files; anything it
	// took from there is replaced by what we decided.
	cfg.Fallbacks = nil
	cfg.RuntimeParams = map[string]string{"application_name": "gp-sync"}
	cfg.ConnectTimeout = connectTimeout
	// Statements are sent unnamed, so a pooler such as Neon's can sit in between.
	cfg.DefaultQueryExecMode = pgx.QueryExecModeExec
	if p.opts.PlaintextForTest {
		cfg.TLSConfig = nil
	} else {
		// The certificate is checked against the NAME, while the dial goes to the checked address.
		cfg.TLSConfig = &tls.Config{ServerName: t.Host, MinVersion: tls.VersionTLS12}
	}
	c, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		oe := &OpenError{Stage: "connect"}
		var pe *pgconn.PgError
		if errors.As(err, &pe) {
			oe.Code = pe.Code
		}
		return nil, oe
	}
	return &conn{c: c}, nil
}

type conn struct{ c *pgx.Conn }

func (c *conn) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = c.c.Close(ctx)
}

// readOnly runs fn in a read-only transaction with a statement timeout. Both
// are set per transaction, not per session, so they also hold behind a pooler.
func (c *conn) readOnly(ctx context.Context, fn func(pgx.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, statementTimeout+5*time.Second)
	defer cancel()
	tx, err := c.c.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = %d", statementTimeout.Milliseconds())); err != nil {
		return err
	}
	return fn(tx)
}

func (c *conn) Test(ctx context.Context) (info TestInfo, err error) {
	err = c.readOnly(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `SHOW server_version`).Scan(&info.ServerVersion); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			SELECT coalesce((SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()), false)`).Scan(&info.TLS); err != nil {
			return err
		}
		// Any write right on a table or view outside the system schemas, or on
		// the public schema itself, means the login is not read only.
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			  WHERE c.relkind IN ('r','v','p') AND n.nspname NOT IN ('pg_catalog','information_schema')
			    AND has_table_privilege(c.oid, 'INSERT,UPDATE,DELETE,TRUNCATE'))
			OR EXISTS (SELECT 1 FROM pg_roles WHERE rolname = current_user AND (rolsuper OR rolcreatedb OR rolcreaterole))`).
			Scan(&info.CanWrite)
	})
	return info, err
}

func (c *conn) Tables(ctx context.Context) (out []Table, err error) {
	err = c.readOnly(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT table_schema, table_name, table_type FROM information_schema.tables
			WHERE table_schema NOT IN ('pg_catalog', 'information_schema') AND table_schema NOT LIKE 'pg\_%'
			  AND table_type IN ('BASE TABLE', 'VIEW')
			  AND has_table_privilege(quote_ident(table_schema) || '.' || quote_ident(table_name), 'SELECT')
			ORDER BY table_schema, table_name LIMIT $1`, maxTables)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t Table
			var kind string
			if err := rows.Scan(&t.Schema, &t.Name, &kind); err != nil {
				return err
			}
			// Names that are not plain lower-case can never be mapped, so they are not offered.
			if IsIdent(t.Schema) && IsIdent(t.Name) {
				t.Kind = "TABLE"
				if kind == "VIEW" {
					t.Kind = "VIEW"
				}
				out = append(out, t)
			}
		}
		return rows.Err()
	})
	return out, err
}

func (c *conn) Columns(ctx context.Context, t TableName) (out []Column, err error) {
	err = c.readOnly(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT column_name, data_type FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position LIMIT $3`, t.Schema, t.Name, maxColumns)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var col Column
			if err := rows.Scan(&col.Name, &col.Type); err != nil {
				return err
			}
			if IsIdent(col.Name) {
				out = append(out, col)
			}
		}
		return rows.Err()
	})
	return out, err
}

// ReadSpec describes one read. Every name in it is checked against
// information_schema before it is put into SQL, and then quoted (SQLI-02).
type ReadSpec struct {
	Table   TableName
	Columns []string // selected, in this order
	OrderBy string   // one of Columns
	Limit   int

	// Change, when set, makes the read incremental: rows whose Change column is at
	// or after Since (all rows when Since is nil), oldest change first. The change
	// time of each row comes back as one extra last value in the row, in UTC as
	// RFC 3339 text, whatever the column's own type. "At or after" re-reads the
	// rows of the last change time on purpose; applying a row twice does nothing.
	Change string
	Since  *time.Time
	// Only limits the read to rows whose OrderBy column, as text, is in the list.
	Only []string
}

func (c *conn) Read(ctx context.Context, spec ReadSpec, fn func(Row) error) error {
	have, err := c.Columns(ctx, spec.Table)
	if err != nil {
		return err
	}
	query, args, err := spec.sql(have)
	if err != nil {
		return err
	}
	return c.readOnly(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			row := make(Row, spec.width())
			dest := make([]any, len(row))
			for i := range row {
				dest[i] = &row[i]
			}
			if err := rows.Scan(dest...); err != nil {
				return err
			}
			if err := fn(row); err != nil {
				return err
			}
		}
		return rows.Err()
	})
}

// width is the number of values in a row: the columns, and the change time when asked for.
func (s ReadSpec) width() int {
	if s.Change != "" {
		return len(s.Columns) + 1
	}
	return len(s.Columns)
}

// sql builds the SELECT and its arguments. Names the table does not have are
// refused; the rest are quoted with pgx.Identifier. Values come back as text so
// one code path reads numbers, booleans and timestamps alike. Values from the
// caller (the time, the ids) are always arguments, never part of the text.
func (s ReadSpec) sql(have []Column) (string, []any, error) {
	known := make(map[string]bool, len(have))
	for _, h := range have {
		known[h.Name] = true
	}
	if len(s.Columns) == 0 || s.Limit < 1 || !known[s.OrderBy] || !IsIdent(s.Table.Schema) || !IsIdent(s.Table.Name) ||
		(s.Change != "" && (!IsIdent(s.Change) || !known[s.Change])) {
		return "", nil, errors.New("source: bad read spec")
	}
	cols := make([]string, 0, len(s.Columns)+1)
	for _, name := range s.Columns {
		if !IsIdent(name) || !known[name] {
			return "", nil, errors.New("source: unknown column")
		}
		cols = append(cols, pgx.Identifier{name}.Sanitize()+"::text")
	}
	var args []any
	var where []string
	order := pgx.Identifier{s.OrderBy}.Sanitize()
	if s.Change != "" {
		at := pgx.Identifier{s.Change}.Sanitize() + "::timestamptz"
		cols = append(cols, `to_char(`+at+` AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`)
		if s.Since != nil {
			args = append(args, s.Since.UTC())
			where = append(where, fmt.Sprintf("%s >= $%d", at, len(args)))
		}
		order = at + ", " + order
	}
	if len(s.Only) > 0 {
		args = append(args, s.Only)
		where = append(where, fmt.Sprintf("%s::text = ANY($%d::text[])", pgx.Identifier{s.OrderBy}.Sanitize(), len(args)))
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	return fmt.Sprintf("SELECT %s FROM %s%s ORDER BY %s LIMIT %d",
		strings.Join(cols, ", "), pgx.Identifier{s.Table.Schema, s.Table.Name}.Sanitize(), clause, order, s.Limit), args, nil
}
