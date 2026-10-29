package payment

import (
	"context"
	"time"

	"gp/internal/shared/db"
)

// FailedBetween counts the payments that failed in [from, to), for the admin's
// dashboard. It reads a number only.
func FailedBetween(ctx context.Context, tx *db.Tx, from, to time.Time) (n int, err error) {
	err = tx.QueryRow(ctx, `SELECT count(*) FROM payments WHERE status = 'FAILED' AND created_at >= $1 AND created_at < $2`, from, to).Scan(&n)
	return n, err
}
