package order

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// What the refund module may do with an order. Each call runs in the refund's
// own transaction and only the order module touches its tables.

// Refunds is what T19 needs from the refund module. *refund.Service is the real
// one. It approves refundID, or, when that is Nil, writes the approved refund of
// what has not been refunded yet.
type Refunds interface {
	ApproveCancel(ctx context.Context, tx *db.Tx, orderID, shopID, refundID uuid.UUID, totalCents money.Cents, actor uuid.UUID, reason string) error
}

// SetRefunds gives the service its refund module. It is a setter because the
// refund module needs the order service too.
func (s *Service) SetRefunds(r Refunds) { s.refunds = r }

// RefundOrder is the order as a refund sees it.
type RefundOrder struct {
	ShopID     uuid.UUID
	Code       string
	Status     enum.OrderStatus
	Version    int
	TotalCents money.Cents
}

// RefundOrder locks the order for the refund. An order the scope cannot see is
// NOT_FOUND, like any other.
func (s *Service) RefundOrder(ctx context.Context, tx *db.Tx, scope db.Scope, orderID uuid.UUID) (RefundOrder, error) {
	o, found, err := lockOrder(ctx, tx, orderID)
	if err != nil || !found || !inScope(scope, o) {
		return RefundOrder{}, apperr.NotFoundUnless(err, found && inScope(scope, o))
	}
	out := RefundOrder{ShopID: o.ShopID, Code: o.Code, Status: o.Status, Version: o.Version}
	err = tx.QueryRow(ctx, `SELECT total_cents FROM orders WHERE id = $1`, orderID).Scan(&out.TotalCents)
	return out, err
}

// RefundLine is a line of the current bill: what the customer paid for it and the
// shop's share of that (its own price times the quantity); the rest is the
// platform's margin.
type RefundLine struct {
	LineCents     money.Cents
	ShopPartCents money.Cents
	state         string
}

func lineOf(ctx context.Context, tx *db.Tx, o orderRow, itemID uuid.UUID) (RefundLine, error) {
	var l RefundLine
	err := tx.QueryRow(ctx, `
		SELECT line_cents, (shop_price_cents * qty)::bigint, state FROM order_items
		WHERE id = $1 AND order_id = $2 AND revision_no = $3`, itemID, o.ID, o.RevisionNo).Scan(&l.LineCents, &l.ShopPartCents, &l.state)
	return l, err
}

// LineOf reads a line of the order's current bill for a refund that is paid out.
func (s *Service) LineOf(ctx context.Context, tx *db.Tx, orderID, itemID uuid.UUID) (RefundLine, error) {
	o, found, err := lockOrder(ctx, tx, orderID)
	if err != nil || !found {
		return RefundLine{}, apperr.NotFoundUnless(err, found)
	}
	l, err := lineOf(ctx, tx, o, itemID)
	return l, apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
}

// MarkLineMissing is the shop's partial refund request (FR-RFD-02): while the
// order is being picked, one line that is still on the bill is marked missing.
// The version goes up, so a screen that showed the line cannot act on it.
func (s *Service) MarkLineMissing(ctx context.Context, tx *db.Tx, scope db.Scope, orderID, itemID uuid.UUID) (RefundLine, error) {
	o, found, err := lockOrder(ctx, tx, orderID)
	if err != nil || !found || !inScope(scope, o) {
		return RefundLine{}, apperr.NotFoundUnless(err, found && inScope(scope, o))
	}
	if o.Status != enum.OrderStatusPicking {
		return RefundLine{}, apperr.New(apperr.InvalidTransition)
	}
	l, err := lineOf(ctx, tx, o, itemID)
	if err != nil {
		if err = db.IgnoreNoRows(err); err == nil {
			err = apperr.Validation(map[string]string{"orderItemId": "unknown"})
		}
		return RefundLine{}, err
	}
	if l.state != "ACTIVE" {
		return RefundLine{}, apperr.Validation(map[string]string{"orderItemId": "not_active"})
	}
	return l, s.setLine(ctx, tx, orderID, itemID, "MISSING")
}

// RestoreLine puts a line back on the bill when its partial refund is rejected.
func (s *Service) RestoreLine(ctx context.Context, tx *db.Tx, orderID, itemID uuid.UUID) error {
	return s.setLine(ctx, tx, orderID, itemID, "ACTIVE")
}

func (s *Service) setLine(ctx context.Context, tx *db.Tx, orderID, itemID uuid.UUID, state string) error {
	if _, err := tx.Exec(ctx, `UPDATE order_items SET state = $3 WHERE id = $1 AND order_id = $2`, itemID, orderID, state); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE orders SET version = version + 1 WHERE id = $1`, orderID)
	return err
}

// CancelRefund is T19 for an approved cancel request: the order moves to
// CANCELLED_REFUND and the hook approves refundID in the same transaction. The
// admin decides on the refund, not on a version, so the current one is used.
func (s *Service) CancelRefund(ctx context.Context, tx *db.Tx, scope db.Scope, orderID, refundID uuid.UUID, reason string) error {
	o, found, err := lockOrder(ctx, tx, orderID)
	if err != nil || !found || !inScope(scope, o) {
		return apperr.NotFoundUnless(err, found && inScope(scope, o))
	}
	return s.transition(ctx, tx, scope, orderID, enum.OrderStatusCancelledRefund, TransitionInput{Version: o.Version, Reason: reason, refundID: refundID})
}
