// Package payment owns the payment attempts of orders (docs/09 section 2,
// docs/07 section 12). It starts a payment with the shop's provider, checks the
// provider's callback, and asks the order module to make the move to PAID. It
// never changes an order's status itself.
package payment

import (
	"context"
	"errors"
	"net/url"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shop"
)

// Payment is one attempt to pay an order. A new one is made for every attempt.
type Payment struct {
	ID          uuid.UUID
	OrderID     uuid.UUID
	ShopID      uuid.UUID
	Provider    enum.PaymentProvider
	AccountID   *uuid.UUID
	AmountCents money.Cents
	Status      enum.PaymentStatus
	ProviderRef *string // the provider's own id for the payment, once it reported one
}

// Provider starts a payment. What it returns goes to the browser as JSON, so it
// must hold nothing secret.
type Provider interface {
	Checkout(ctx context.Context, acc shop.PayAccount, p Payment, o order.Payable) (any, error)
}

// Notifier is a provider that tells us about a payment by calling us (PayHere).
// ParseNotify checks everything the provider sent (signature, merchant, currency
// and amount) and returns only what was verified.
type Notifier interface {
	ParseNotify(ctx context.Context, acc shop.PayAccount, p Payment, form url.Values) (Notified, error)
}

// Notified is a verified callback.
type Notified struct {
	// Ref is the provider's id of the payment: the key that makes a replay a no-op.
	Ref string
	// Code is the provider's status code (PayHere: 2 success, 0 pending, -1 cancelled, -2 failed, -3 charged back).
	Code   int
	Method string
	// Raw is the callback without card data and without the signature.
	Raw map[string]string
}

var (
	// ErrBadSignature is a callback that does not carry our provider's signature.
	ErrBadSignature = errors.New("payment: bad signature")
	// ErrMismatch is a correctly signed callback that does not match the payment
	// (another merchant, currency or amount). It never pays an order.
	ErrMismatch = errors.New("payment: callback does not match the payment")
	// ErrNoBank is a manual account with no bank details to show.
	ErrNoBank = errors.New("payment: the account has no bank details")
)

// Registry picks the provider by the name an order carries.
type Registry map[enum.PaymentProvider]Provider
