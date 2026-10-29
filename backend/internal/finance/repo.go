package finance

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/ids"
)

// insert adds one row. orderID is Nil for a row that belongs to no order (a payout).
func insert(ctx context.Context, tx *db.Tx, shopID, orderID uuid.UUID, e entry) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (shop_id, order_id, refund_id, payout_id, type, party, amount_cents, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''))`, shopID, ids.Ref(orderID), e.refundID, e.payoutID, e.kind, e.party, e.amount, e.note)
	return err
}
