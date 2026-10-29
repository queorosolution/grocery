// Package manual is the bank transfer provider: the customer is told where to
// send the money, and an admin marks the order paid with the bank's reference.
package manual

import (
	"context"

	"gp/internal/order"
	"gp/internal/payment"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shop"
)

type Provider struct{}

func New() *Provider { return &Provider{} }

// Checkout is what the customer sees (docs/06 section 6).
type Checkout struct {
	Provider    enum.PaymentProvider `json:"provider"`
	Bank        shop.Bank            `json:"bank"`
	Reference   string               `json:"reference"` // the order code: the customer writes it on the transfer
	AmountCents money.Cents          `json:"amountCents"`
}

// Checkout needs the bank details of the account; one without them cannot be paid into.
func (Provider) Checkout(_ context.Context, acc shop.PayAccount, p payment.Payment, o order.Payable) (any, error) {
	if acc.Bank == nil {
		return nil, payment.ErrNoBank
	}
	return Checkout{Provider: enum.PaymentProviderManual, Bank: *acc.Bank, Reference: o.Code, AmountCents: p.AmountCents}, nil
}
