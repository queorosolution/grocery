package order

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// OpenWork tells the account module whether the customer still has an order in
// progress: a bill not answered or not paid, or one being packed or delivered.
// Such an account cannot be closed yet (FR-AUTH-09).
func (s *Service) OpenWork(ctx context.Context, tx *db.Tx, userID uuid.UUID) (map[string]string, error) {
	var live []string
	for _, st := range enum.OrderStatusValues() {
		if !IsFinal(st) {
			live = append(live, string(st))
		}
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM orders WHERE customer_id = $1 AND status = ANY($2)`, userID, live).Scan(&n); err != nil {
		return nil, err
	}
	if n > 0 {
		return map[string]string{"orders": "open"}, nil
	}
	return nil, nil
}
