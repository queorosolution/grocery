package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"gp/internal/shared/logx"
)

// Querier is what repos accept. Only *Tx implements it, so SQL can run only
// inside a transaction that has a Scope.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Tx is an open transaction. It has no Commit or Rollback: DB.Tx owns both.
type Tx struct {
	tx    pgx.Tx
	after []func(context.Context) error
}

var _ Querier = (*Tx)(nil)

func (t *Tx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return t.tx.Exec(ctx, sql, args...)
}

func (t *Tx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return t.tx.Query(ctx, sql, args...)
}

func (t *Tx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return t.tx.QueryRow(ctx, sql, args...)
}

// SendBatch runs several statements in one round trip.
func (t *Tx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return t.tx.SendBatch(ctx, b)
}

// AfterCommit queues work (for example publishing events to SQS) to run once,
// after the transaction commits. If the transaction rolls back, it never runs.
func (t *Tx) AfterCommit(fn func(context.Context) error) { t.after = append(t.after, fn) }

// DB is the only way to open a transaction.
type DB struct{ pool *pgxpool.Pool }

// Pool exposes the pool for health checks and tests. Application code must use Tx.
func (d *DB) Pool() *pgxpool.Pool { return d.pool }

func (d *DB) Close() { d.pool.Close() }

// Tx runs fn in one transaction for the given scope. It commits when fn returns
// nil, rolls back on an error or a panic (the panic continues), then runs the
// AfterCommit hooks in order.
func (d *DB) Tx(ctx context.Context, scope Scope, fn func(ctx context.Context, tx *Tx) error) (err error) {
	raw, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}
	tx := &Tx{tx: raw}
	committed := false
	defer func() {
		// Roll back unless we committed. This also covers a panic and a callback
		// that ends its goroutine (runtime.Goexit, e.g. t.Fatal in a test), which
		// would otherwise leave the connection stuck in an open transaction. A
		// cancelled request context must not leave it open either.
		if !committed {
			_ = raw.Rollback(context.WithoutCancel(ctx))
		}
	}()

	// true = local to this transaction, so the values cannot leak to the next
	// request that reuses the pooled connection.
	user, shop, privileged := scope.settings()
	if _, err = raw.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.shop_id', $2, true), set_config('app.privileged', $3, true)`,
		user, shop, privileged); err != nil {
		return fmt.Errorf("db: set scope: %w", err)
	}
	if err = fn(ctx, tx); err != nil {
		return err
	}
	if err = raw.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit: %w", err)
	}
	committed = true
	d.runAfterCommit(ctx, tx.after)
	return nil
}

// runAfterCommit cannot undo the commit, so a failing hook is logged, not
// returned. The caller's context is detached: a client that hangs up after the
// commit must not cancel the event publish. The outbox sweep re-sends anything
// a hook missed (D17).
func (d *DB) runAfterCommit(ctx context.Context, hooks []func(context.Context) error) {
	ctx = context.WithoutCancel(ctx)
	for _, hook := range hooks {
		if err := hook(ctx); err != nil {
			logx.From(ctx).Error("after-commit hook failed", "err", err)
		}
	}
}
