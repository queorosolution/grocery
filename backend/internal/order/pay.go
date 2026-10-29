package order

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/user"
)

// What the payment module may ask of an order. The money moves in the payment
// module's transaction, but the status changes only here, through transition.

// Payable is an order that waits for payment, as the payment module needs it to
// start a payment. The totals are the frozen ones.
type Payable struct {
	ID, ShopID uuid.UUID
	Code       string
	Version    int
	TotalCents money.Cents
	Provider   enum.PaymentProvider
	Customer   user.Contact
	Address    string
}

// Payable reads the order of a customer who is about to pay. The order must be
// theirs (else NOT_FOUND), the version the one they saw (STALE_ORDER) and the
// status AWAITING_PAYMENT (INVALID_TRANSITION). The row stays locked until the
// caller's transaction ends, so the total cannot change while a payment starts.
func (s *Service) Payable(ctx context.Context, tx *db.Tx, scope db.Scope, id uuid.UUID, version int) (Payable, error) {
	o, found, err := lockOrder(ctx, tx, id)
	if err != nil || !found || !inScope(scope, o) {
		return Payable{}, apperr.NotFoundUnless(err, found && inScope(scope, o))
	}
	switch {
	case o.Version != version:
		return Payable{}, apperr.New(apperr.StaleOrder)
	case o.Status != enum.OrderStatusAwaitingPayment:
		return Payable{}, apperr.New(apperr.InvalidTransition)
	}
	p := Payable{ID: o.ID, ShopID: o.ShopID, Code: o.Code, Version: o.Version}
	var snap addressSnapshot
	if err := tx.QueryRow(ctx, `SELECT total_cents, payment_provider, address FROM orders WHERE id = $1`, id).
		Scan(&p.TotalCents, &p.Provider, &snap); err != nil {
		return p, err
	}
	if p.Customer, err = s.users.ContactOf(ctx, tx, o.CustomerID); err != nil {
		return p, err
	}
	p.Address = snap.Line1
	if snap.Landmark != nil && *snap.Landmark != "" {
		p.Address += ", " + *snap.Landmark
	}
	return p, nil
}

// PayState is what a payment callback needs to decide: can this order still be
// paid, and for how much.
type PayState struct {
	ShopID     uuid.UUID
	Status     enum.OrderStatus
	Version    int
	TotalCents money.Cents
	Provider   enum.PaymentProvider
}

// PayState reads and locks the order. An order the scope cannot see is NOT_FOUND.
func (s *Service) PayState(ctx context.Context, tx *db.Tx, scope db.Scope, id uuid.UUID) (PayState, error) {
	o, found, err := lockOrder(ctx, tx, id)
	if err != nil || !found || !inScope(scope, o) {
		return PayState{}, apperr.NotFoundUnless(err, found && inScope(scope, o))
	}
	st := PayState{ShopID: o.ShopID, Status: o.Status, Version: o.Version}
	err = tx.QueryRow(ctx, `SELECT total_cents, payment_provider FROM orders WHERE id = $1`, id).Scan(&st.TotalCents, &st.Provider)
	return st, err
}

// MarkPaid is T10 inside the caller's transaction. The payment row, the move and
// the ledger commit together or not at all. Only the system (a verified
// callback) and the super admin may make it; the table decides.
func (s *Service) MarkPaid(ctx context.Context, tx *db.Tx, scope db.Scope, id uuid.UUID, version int) error {
	return s.transition(ctx, tx, scope, id, enum.OrderStatusPaid, TransitionInput{Version: version})
}
