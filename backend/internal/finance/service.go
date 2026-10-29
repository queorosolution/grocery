// Package finance owns the ledger (docs/05 section 4): every money movement is
// a signed row, written in the transaction of the status change that causes it.
// Rows are never edited or deleted.
package finance

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// Service posts ledger rows. It keeps no state: it always works inside the
// caller's transaction.
type Service struct{}

func NewService() *Service { return &Service{} }

// PaidFacts are the frozen numbers of an order that was just paid.
type PaidFacts struct {
	OrderID, ShopID uuid.UUID
	ItemsCents      money.Cents // what the customer pays for the items (online prices)
	MarginCents     money.Cents // the platform's part of the items
	ServiceFeeCents money.Cents
	DiscountCents   money.Cents
	// FundedBy is who carries the discount; it is empty when there is none.
	FundedBy enum.FundedBy
}

type entry struct {
	kind     enum.LedgerType
	party    enum.LedgerParty
	amount   money.Cents
	note     string
	refundID *uuid.UUID // set on the rows a refund writes
	payoutID *uuid.UUID // set on the row a payout writes
}

var errFacts = errors.New("finance: the order numbers do not add up")

// paidEntries turns an order's numbers into its T10 rows. Rows of 0 are left
// out. The shop's part is the items without the platform's margin; a discount
// is a negative row for whoever funds it.
func paidEntries(f PaidFacts) ([]entry, error) {
	if f.MarginCents < 0 || f.MarginCents > f.ItemsCents || f.ServiceFeeCents < 0 || f.DiscountCents < 0 || f.DiscountCents > f.ItemsCents {
		return nil, errFacts
	}
	rows := []entry{
		{kind: enum.LedgerTypeSale, party: enum.LedgerPartyShop, amount: f.ItemsCents - f.MarginCents},
		{kind: enum.LedgerTypePlatformMargin, party: enum.LedgerPartyPlatform, amount: f.MarginCents},
		{kind: enum.LedgerTypeServiceFee, party: enum.LedgerPartyPlatform, amount: f.ServiceFeeCents},
	}
	if f.DiscountCents > 0 {
		var party enum.LedgerParty
		switch f.FundedBy {
		case enum.FundedByPlatform:
			party = enum.LedgerPartyPlatform
		case enum.FundedByShop:
			party = enum.LedgerPartyShop
		default:
			return nil, errFacts
		}
		rows = append(rows, entry{kind: enum.LedgerTypeDiscount, party: party, amount: -f.DiscountCents, note: "coupon"})
	}
	return rows, nil
}

// PostPaid writes the rows of T10: sale, platform margin, service fee and discount.
// The delivery fee comes later, when the order is delivered.
func (s *Service) PostPaid(ctx context.Context, tx *db.Tx, f PaidFacts) error {
	rows, err := paidEntries(f)
	if err != nil {
		return err
	}
	for _, e := range rows {
		if e.amount == 0 {
			continue
		}
		if err := insert(ctx, tx, f.ShopID, f.OrderID, e); err != nil {
			return err
		}
	}
	return nil
}

// DeliveredFacts are the numbers of an order that was just delivered.
type DeliveredFacts struct {
	OrderID, ShopID  uuid.UUID
	DeliveryFeeCents money.Cents
	// ByShopDriver is true when the shop's own driver delivered, so the fee is the
	// shop's; else a platform driver did and it is the platform's.
	ByShopDriver bool
}

// PostDelivered writes the row of T16: the delivery fee, for whoever delivered
// (docs/05 section 4). A free delivery posts nothing.
func (s *Service) PostDelivered(ctx context.Context, tx *db.Tx, f DeliveredFacts) error {
	if f.DeliveryFeeCents < 0 {
		return errFacts
	}
	if f.DeliveryFeeCents == 0 {
		return nil
	}
	party := enum.LedgerPartyPlatform
	if f.ByShopDriver {
		party = enum.LedgerPartyShop
	}
	return insert(ctx, tx, f.ShopID, f.OrderID, entry{kind: enum.LedgerTypeDeliveryFee, party: party, amount: f.DeliveryFeeCents})
}
