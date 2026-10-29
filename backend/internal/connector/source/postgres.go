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
}

func (c *conn) Read(ctx context.Context, spec ReadSpec, fn func(Row) error) error {
	have, err := c.Columns(ctx, spec.Table)
	if err != nil {
		return err
	}
	query, err := spec.sql(have)
	if err != nil {
		return err
	}
	return c.readOnly(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			row := make(Row, len(spec.Columns))
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

// sql builds the SELECT. Names the table does not have are refused; the rest
// are quoted with pgx.Identifier. Values come back as text so one code path
// reads numbers, booleans and timestamps alike.
func (s ReadSpec) sql(have []Column) (string, error) {
	known := make(map[string]bool, len(have))
	for _, h := range have {
		known[h.Name] = true
	}
	if len(s.Columns) == 0 || s.Limit < 1 || !known[s.OrderBy] || !IsIdent(s.Table.Schema) || !IsIdent(s.Table.Name) {
		return "", errors.New("source: bad read spec")
	}
	cols := make([]string, len(s.Columns))
	for i, name := range s.Columns {
		if !IsIdent(name) || !known[name] {
			return "", errors.New("source: unknown column")
		}
		cols[i] = pgx.Identifier{name}.Sanitize() + "::text"
	}
	return fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT %d",
		strings.Join(cols, ", "), pgx.Identifier{s.Table.Schema, s.Table.Name}.Sanitize(), pgx.Identifier{s.OrderBy}.Sanitize(), s.Limit), nil
}
