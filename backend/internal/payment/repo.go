package payment

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

const paymentColumns = `id, order_id, shop_id, provider, account_id, amount_cents, status, provider_ref`

type scanner interface{ Scan(...any) error }

func scanPayment(row scanner) (Payment, error) {
	var p Payment
	err := row.Scan(&p.ID, &p.OrderID, &p.ShopID, &p.Provider, &p.AccountID, &p.AmountCents, &p.Status, &p.ProviderRef)
	return p, err
}

// insertAttempt adds an INITIATED payment for an order.
func insertAttempt(ctx context.Context, tx *db.Tx, orderID, shopID uuid.UUID, provider enum.PaymentProvider, accountID uuid.UUID, amount money.Cents) (Payment, error) {
	return scanPayment(tx.QueryRow(ctx, `
		INSERT INTO payments (order_id, shop_id, provider, account_id, amount_cents)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+paymentColumns, orderID, shopID, provider, accountID, amount))
}

// paymentByID reads a payment without a lock, for the checks before the callback is acted on.
func paymentByID(ctx context.Context, tx *db.Tx, id uuid.UUID) (Payment, bool, error) {
	p, err := scanPayment(tx.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id))
	return p, err == nil, db.IgnoreNoRows(err)
}

// lockPayment reads a payment and holds it until the transaction ends, so two
// callbacks for one payment run one after the other.
func lockPayment(ctx context.Context, tx *db.Tx, id uuid.UUID) (Payment, bool, error) {
	p, err := scanPayment(tx.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE id = $1 FOR UPDATE`, id))
	return p, err == nil, db.IgnoreNoRows(err)
}

// settle records what a callback said. The provider reference is only ever set
// once (the unique index on provider and reference turns a second payment with
// the same reference into a unique violation).
func settle(ctx context.Context, tx *db.Tx, id uuid.UUID, status enum.PaymentStatus, ref, method string, raw map[string]string) error {
	body, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		UPDATE payments SET status = $2, provider_ref = COALESCE(NULLIF($3, ''), provider_ref),
		       method = COALESCE(NULLIF($4, ''), method), raw = $5::jsonb
		WHERE id = $1`, id, status, ref, method, string(body))
	return err
}

// settleManual closes the manual attempts of an order when an admin confirms the
// transfer: the newest open one becomes the paid one, the others are cancelled.
// With none open, the payment is recorded as a new row. It returns the paid
// payment id.
func settleManual(ctx context.Context, tx *db.Tx, orderID, shopID uuid.UUID, amount money.Cents, reference string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		UPDATE payments SET status = 'SUCCESS', provider_ref = $2, method = 'BANK_TRANSFER', amount_cents = $3
		WHERE id = (SELECT id FROM payments WHERE order_id = $1 AND provider = 'MANUAL' AND status = 'INITIATED'
		            ORDER BY created_at DESC, id DESC LIMIT 1)
		RETURNING id`, orderID, reference, amount).Scan(&id)
	if err = db.IgnoreNoRows(err); err != nil {
		return uuid.Nil, err
	}
	if id == uuid.Nil {
		err = tx.QueryRow(ctx, `
			INSERT INTO payments (order_id, shop_id, provider, amount_cents, status, provider_ref, method)
			VALUES ($1, $2, 'MANUAL', $3, 'SUCCESS', $4, 'BANK_TRANSFER') RETURNING id`, orderID, shopID, amount, reference).Scan(&id)
		return id, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE payments SET status = 'CANCELLED'
		WHERE order_id = $1 AND provider = 'MANUAL' AND status = 'INITIATED' AND id <> $2`, orderID, id)
	return id, err
}
