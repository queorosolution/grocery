package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AppRole is the only role the application may connect as. It has no
// BYPASSRLS, so row-level security applies to it (INFRA-05).
const AppRole = "app_rw"

// maxConns is per Lambda instance. Neon's pooler fans these in; more would
// exhaust its connection limit under concurrent Lambdas.
const maxConns = 2

// Options controls Open.
type Options struct {
	URL string
	// Local relaxes the TLS and role checks for a developer's own Postgres.
	Local bool
}

// Open connects, then refuses to return a pool that is unsafe to use.
func Open(ctx context.Context, opt Options) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(opt.URL)
	if err != nil {
		return nil, errors.New("db: invalid database URL") // the URL holds the password: never echo it
	}
	if !opt.Local {
		if err := requireTLS(cfg); err != nil {
			return nil, err
		}
	}
	cfg.MaxConns = maxConns
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 30 * time.Second
	// Neon's pooler runs in transaction mode and cannot keep named prepared
	// statements; this mode sends each query on its own and works with it.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}
	if err := CheckRole(ctx, pool, opt.Local); err != nil {
		pool.Close()
		return nil, err
	}
	return &DB{pool: pool}, nil
}

// requireTLS refuses sslmode=disable, allow and prefer: all of them can end up
// without encryption.
func requireTLS(cfg *pgxpool.Config) error {
	if cfg.ConnConfig.TLSConfig == nil || len(cfg.ConnConfig.Fallbacks) > 0 {
		return errors.New("db: the database URL must use sslmode=require (or verify-full)")
	}
	return nil
}

// CheckRole is the start-up check from INFRA-05: connecting as the owner skips
// row-level security, so the app must not start that way. Open runs it; it is
// exported so tests can run it against any pool.
func CheckRole(ctx context.Context, pool *pgxpool.Pool, local bool) error {
	var user string
	var bypass bool
	err := pool.QueryRow(ctx, `
		SELECT current_user, r.rolsuper OR r.rolbypassrls
		FROM pg_roles r WHERE r.rolname = current_user`).Scan(&user, &bypass)
	if err != nil {
		return fmt.Errorf("db: check role: %w", err)
	}
	if local {
		return nil
	}
	if user != AppRole {
		return fmt.Errorf("db: connected as %q, the app must connect as %q (the owner skips row-level security)", user, AppRole)
	}
	if bypass {
		return fmt.Errorf("db: role %q can bypass row-level security; remove SUPERUSER and BYPASSRLS", AppRole)
	}
	return nil
}
