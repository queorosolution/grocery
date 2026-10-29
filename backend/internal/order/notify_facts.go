package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
)

// NotifyFacts is what a text about an order needs. It holds no address and no
// price: the texts never carry them.
type NotifyFacts struct {
	ID         uuid.UUID
	Code       string
	ShopID     uuid.UUID
	CustomerID uuid.UUID
	Items      int    // lines that still count
	Slot       string // "30 Oct 14:00-16:00", or "courier" when the order has no slot
}

// slotText writes a window in Colombo time, short enough for an SMS.
func slotText(starts, ends *time.Time) string {
	if starts == nil || ends == nil {
		return "courier"
	}
	s, e := clock.Colombo(*starts), clock.Colombo(*ends)
	return s.Format("2 Jan 15:04") + "-" + e.Format("15:04")
}

// NotifyFacts reads them inside the caller's transaction (the notify consumers
// run as the system). An unknown order is NOT_FOUND.
func (s *Service) NotifyFacts(ctx context.Context, tx *db.Tx, id uuid.UUID) (NotifyFacts, error) {
	f := NotifyFacts{ID: id}
	var starts, ends *time.Time
	err := tx.QueryRow(ctx, `
		SELECT o.code, o.shop_id, o.customer_id, sl.starts_at, sl.ends_at,
		       (SELECT count(*) FROM order_items i WHERE i.order_id = o.id AND i.revision_no = o.revision_no AND i.state IN ('ACTIVE', 'SUBSTITUTE'))
		FROM orders o LEFT JOIN slots sl ON sl.id = o.slot_id WHERE o.id = $1`, id).
		Scan(&f.Code, &f.ShopID, &f.CustomerID, &starts, &ends, &f.Items)
	if err != nil {
		return f, apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
	}
	f.Slot = slotText(starts, ends)
	return f, nil
}

// DeliveryCodeOf is the code for the customer's text. It is read only while the
// order is out for delivery, and it goes to the customer and nobody else.
func (s *Service) DeliveryCodeOf(ctx context.Context, tx *db.Tx, id uuid.UUID) (string, error) {
	var out bool
	if err := tx.QueryRow(ctx, `SELECT status = 'OUT_FOR_DELIVERY' FROM orders WHERE id = $1`, id).Scan(&out); err != nil {
		return "", apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
	}
	if !out {
		return "", apperr.New(apperr.InvalidTransition)
	}
	code, _, ok, err := s.codeOf(ctx, tx, id)
	if err != nil || !ok {
		return "", apperr.NotFoundUnless(err, ok)
	}
	return code, nil
}
