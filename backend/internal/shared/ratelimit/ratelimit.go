// Package ratelimit is a fixed-window counter kept in Postgres (rate_counters),
// so every Lambda instance sees the same count. Callers build the key; put a
// hash in it, never an email, phone number or raw IP.
package ratelimit

import (
	"context"
	"time"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
)

// Hit counts one attempt for key and returns RATE_LIMITED once more than limit
// attempts happened in the current window. It runs in its own committed
// transaction on purpose: inside the caller's transaction a refused request
// would roll the counter back and the limit would never trigger. The attempt is
// counted even when refused, so a client that keeps pushing stays blocked.
func Hit(ctx context.Context, d *db.DB, key string, limit int, window time.Duration) error {
	start := clock.Now().Truncate(window)
	var count int
	err := d.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO rate_counters (key, window_start, count) VALUES ($1, $2, 1)
			ON CONFLICT (key, window_start) DO UPDATE SET count = rate_counters.count + 1
			RETURNING count`, key, start).Scan(&count)
	})
	if err != nil {
		return apperr.New(apperr.Internal).Wrap(err)
	}
	if count > limit {
		return apperr.New(apperr.RateLimited)
	}
	return nil
}
