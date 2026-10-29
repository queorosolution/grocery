package delivery

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
)

// NextSlots is the start of the soonest slot a customer could still book, for
// each shop that has one (open, before its cutoff, not full).
func (s *Service) NextSlots(ctx context.Context, tx *db.Tx, now time.Time) (map[uuid.UUID]time.Time, error) {
	rows, err := tx.Query(ctx, `
		SELECT shop_id, min(starts_at) FROM slots
		WHERE status = 'OPEN' AND cutoff_at > $1 AND booked < capacity GROUP BY shop_id`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]time.Time{}
	for rows.Next() {
		var id uuid.UUID
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		out[id] = at
	}
	return out, rows.Err()
}
