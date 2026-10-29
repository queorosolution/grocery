package shop

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
)

// OpenAt reports whether the shop is open at t (Colombo time). A shop without
// hours is closed. It is for callers that act only in open hours, such as the
// catalog sync (NFR-COST-02).
func (s *Service) OpenAt(ctx context.Context, tx *db.Tx, shopID uuid.UUID, t time.Time) (bool, error) {
	days, err := hoursOf(ctx, tx, shopID)
	if err != nil {
		return false, err
	}
	return weekOf(days).openAt(t), nil
}
