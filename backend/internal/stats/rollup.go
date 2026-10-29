package stats

import (
	"context"
	"time"

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

// retention is one clean-up: a name for the log, how long the rows stay, and a
// delete that takes the cut-off time as $1.
var retention = []struct {
	name string
	keep time.Duration
	sql  string
}{
	{"sessions", keepSessions, `DELETE FROM sessions WHERE expires_at < $1 OR revoked_at < $1`},
	{"otp_codes", keepShort, `DELETE FROM otp_codes WHERE created_at < $1`},
	{"rate_counters", keepShort, `DELETE FROM rate_counters WHERE window_start < $1`},
	{"connector_runs", keepConnectorRuns, `DELETE FROM connector_runs WHERE started_at < $1`},
	{"notifications", keepNotifications, `DELETE FROM notifications WHERE created_at < $1`},
}

// Rollup is the nightly job (docs/04 section 6): one database wake per day. The
// day's numbers are already exact, because each event is added once, so
// "closing" yesterday needs no sum; the job only deletes what is past its
// retention. It returns the rows deleted per table.
func (s *Service) Rollup(ctx context.Context) (map[string]int64, error) {
	out := make(map[string]int64, len(retention))
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		for _, r := range retention {
			tag, err := tx.Exec(ctx, r.sql, clock.Now().Add(-r.keep))
			if err != nil {
				return err
			}
			out[r.name] = tag.RowsAffected()
		}
		return nil
	})
	return out, err
}
