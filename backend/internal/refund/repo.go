package refund

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/ids"
	"gp/internal/shared/money"
)

const columns = `r.id, r.order_id, o.code, r.shop_id, r.type, r.order_item_id, r.amount_cents, r.reason, r.status, r.method,
	r.reference, r.requested_by, r.decided_at, r.paid_out_at, r.created_at`

// listLimit is the longest list the admin gets.
const listLimit = 100

func scan(row interface{ Scan(...any) error }) (r Refund, err error) {
	err = row.Scan(&r.ID, &r.OrderID, &r.OrderCode, &r.ShopID, &r.Type, &r.OrderItemID, &r.AmountCents, &r.Reason, &r.Status, &r.Method,
		&r.Reference, &r.RequestedBy, &r.DecidedAt, &r.PaidOutAt, &r.CreatedAt)
	return r, err
}

// newRefund is one row to insert. The payment is the order's successful one
// unless the caller names it.
type newRefund struct {
	OrderID, ShopID uuid.UUID
	PaymentID       uuid.UUID
	Type            enum.RefundType
	ItemID          *uuid.UUID
	AmountCents     money.Cents
	Reason          string
	Status          enum.RefundStatus
	RequestedBy     uuid.UUID
	DecidedBy       uuid.UUID
}

func insert(ctx context.Context, tx *db.Tx, n newRefund) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO refunds (order_id, shop_id, payment_id, type, order_item_id, amount_cents, reason, status, requested_by, decided_by, decided_at)
		VALUES ($1, $2, COALESCE($3, (SELECT id FROM payments WHERE order_id = $1 AND status = 'SUCCESS' ORDER BY created_at LIMIT 1)),
		        $4, $5, $6, $7, $8, $9, $10, CASE WHEN $10::uuid IS NULL THEN NULL ELSE now() END)
		RETURNING id`,
		n.OrderID, n.ShopID, ids.Ref(n.PaymentID), n.Type, n.ItemID, n.AmountCents, n.Reason, n.Status, ids.Ref(n.RequestedBy), ids.Ref(n.DecidedBy)).Scan(&id)
	return id, err
}

// announce tells the admin queue about a new request, in the same transaction.
func announce(ctx context.Context, tx *db.Tx, id uuid.UUID, n newRefund) error {
	_, err := events.Add(ctx, tx, events.Event{Type: enum.EventTypeRefundRequested, ShopID: n.ShopID, EntityID: id.String(),
		Payload: map[string]any{"refundId": id, "orderId": n.OrderID, "type": n.Type, "amountCents": n.AmountCents}})
	return err
}

// get reads and locks one refund.
func get(ctx context.Context, tx *db.Tx, id uuid.UUID) (Refund, bool, error) {
	r, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM refunds r JOIN orders o ON o.id = r.order_id WHERE r.id = $1 FOR UPDATE OF r`, id))
	return r, err == nil, db.IgnoreNoRows(err)
}

// openFor counts the refunds of the order that still count against what the
// customer paid: not rejected, not a late payment (that is another payment).
func openFor(ctx context.Context, tx *db.Tx, orderID, except uuid.UUID) (n int, full bool, requested int, sum money.Cents, err error) {
	err = tx.QueryRow(ctx, `
		SELECT count(*), COALESCE(bool_or(type = 'FULL'), false), count(*) FILTER (WHERE status = 'REQUESTED'), COALESCE(sum(amount_cents), 0)::bigint
		FROM refunds WHERE order_id = $1 AND type <> 'LATE_PAYMENT' AND status <> 'REJECTED' AND id <> $2`, orderID, except).Scan(&n, &full, &requested, &sum)
	return
}
