package finance

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/db"
)

func insert(ctx context.Context, tx *db.Tx, shopID, orderID uuid.UUID, e entry) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ledger_entries (shop_id, order_id, type, party, amount_cents, note)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''))`, shopID, orderID, e.kind, e.party, e.amount, e.note)
	return err
}
