// Package refund owns refund requests (docs/03 section 11). This file holds what
// the payment module needs: opening a request for money that arrived but cannot
// stand. Deciding and paying out come with the refund prompt.
package refund

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

type Service struct{}

func NewService() *Service { return &Service{} }

// OpenInput is a refund the system asks for itself. The reason is fixed text
// written by the caller, never customer input.
type OpenInput struct {
	OrderID, ShopID uuid.UUID
	PaymentID       uuid.UUID
	Type            enum.RefundType
	AmountCents     money.Cents
	Reason          string
}

// Open adds a REQUESTED refund and tells the admin through the outbox, in the
// caller's transaction, so the money and its request are saved together.
func (s *Service) Open(ctx context.Context, tx *db.Tx, in OpenInput) (uuid.UUID, error) {
	if in.AmountCents <= 0 || in.Type == enum.RefundTypePartial {
		return uuid.Nil, fmt.Errorf("refund: cannot open a %s refund of %d", in.Type, in.AmountCents)
	}
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO refunds (order_id, shop_id, payment_id, type, amount_cents, reason)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		in.OrderID, in.ShopID, in.PaymentID, in.Type, in.AmountCents, in.Reason).Scan(&id)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = events.Add(ctx, tx, events.Event{Type: enum.EventTypeRefundRequested, ShopID: in.ShopID, EntityID: id.String(),
		Payload: map[string]any{"refundId": id, "orderId": in.OrderID, "type": in.Type, "amountCents": in.AmountCents}})
	return id, err
}
