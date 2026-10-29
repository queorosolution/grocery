package stats

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
)

// Retention windows (docs/05 section 7). Orders, payments, the ledger, refunds and
// the audit log are kept 7 years and are never touched here. The outbox sweep
// already prunes processed_events and idempotency_keys at 7 days.
const (
	keepShort         = 7 * 24 * time.Hour
	keepSessions      = 30 * 24 * time.Hour
	keepConnectorRuns = 90 * 24 * time.Hour
	keepNotifications = 365 * 24 * time.Hour
)

// retention is one clean-up: a name for the log, how long the rows stay, and the
// delete, written out so its text is fixed. The cut-off time is $1.
var retention = []struct {
	name string
	keep time.Duration
	run  func(ctx context.Context, tx *db.Tx, cutoff time.Time) (pgconn.CommandTag, error)
}{
	{"sessions", keepSessions, func(ctx context.Context, tx *db.Tx, cutoff time.Time) (pgconn.CommandTag, error) {
		return tx.Exec(ctx, `DELETE FROM sessions WHERE expires_at < $1 OR revoked_at < $1`, cutoff)
	}},
	{"otp_codes", keepShort, func(ctx context.Context, tx *db.Tx, cutoff time.Time) (pgconn.CommandTag, error) {
		return tx.Exec(ctx, `DELETE FROM otp_codes WHERE created_at < $1`, cutoff)
	}},
	{"rate_counters", keepShort, func(ctx context.Context, tx *db.Tx, cutoff time.Time) (pgconn.CommandTag, error) {
		return tx.Exec(ctx, `DELETE FROM rate_counters WHERE window_start < $1`, cutoff)
	}},
	{"connector_runs", keepConnectorRuns, func(ctx context.Context, tx *db.Tx, cutoff time.Time) (pgconn.CommandTag, error) {
		return tx.Exec(ctx, `DELETE FROM connector_runs WHERE started_at < $1`, cutoff)
	}},
	{"notifications", keepNotifications, func(ctx context.Context, tx *db.Tx, cutoff time.Time) (pgconn.CommandTag, error) {
		return tx.Exec(ctx, `DELETE FROM notifications WHERE created_at < $1`, cutoff)
	}},
}

// Rollup is the nightly job (docs/04 section 6): one database wake per day. The
// day's numbers are already exact, because each event is added once, so
// "closing" yesterday needs no sum; the job only deletes what is past its
// retention. It returns the rows deleted per table.
func (s *Service) Rollup(ctx context.Context) (map[string]int64, error) {
	out := make(map[string]int64, len(retention))
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		for _, r := range retention {
			tag, err := r.run(ctx, tx, clock.Now().Add(-r.keep))
			if err != nil {
				return err
			}
			out[r.name] = tag.RowsAffected()
		}
		return nil
	})
	return out, err
}
