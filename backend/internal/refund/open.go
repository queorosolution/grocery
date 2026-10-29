package refund

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

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
	n := newRefund{OrderID: in.OrderID, ShopID: in.ShopID, PaymentID: in.PaymentID, Type: in.Type, AmountCents: in.AmountCents,
		Reason: in.Reason, Status: enum.RefundStatusRequested}
	id, err := insert(ctx, tx, n)
	if err != nil {
		return uuid.Nil, err
	}
	return id, announce(ctx, tx, id, n)
}
