package finance

import (
	"testing"

	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

func total(rows []entry) (sum money.Cents) {
	for _, e := range rows {
		sum += e.amount
	}
	return sum
}

func TestPaidEntries(t *testing.T) {
	for name, tc := range map[string]struct {
		f    PaidFacts
		want money.Cents // items + service fee - discount
	}{
		"plain":                 {PaidFacts{ItemsCents: 205500, MarginCents: 5500, ServiceFeeCents: 2000}, 207500},
		"shop funded discount":  {PaidFacts{ItemsCents: 205500, MarginCents: 5500, DiscountCents: 20550, FundedBy: enum.FundedByShop}, 184950},
		"platform funded":       {PaidFacts{ItemsCents: 205500, MarginCents: 5500, ServiceFeeCents: 2000, DiscountCents: 7500, FundedBy: enum.FundedByPlatform}, 200000},
		"all of it discounted":  {PaidFacts{ItemsCents: 1000, MarginCents: 100, DiscountCents: 1000, FundedBy: enum.FundedByShop}, 0},
		"no margin, no charges": {PaidFacts{ItemsCents: 1000}, 1000},
	} {
		rows, err := paidEntries(tc.f)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got := total(rows); got != tc.want {
			t.Errorf("%s: rows add up to %d, want %d", name, got, tc.want)
		}
	}
}

func TestPaidEntries_Parties(t *testing.T) {
	rows, _ := paidEntries(PaidFacts{ItemsCents: 10000, MarginCents: 2500, ServiceFeeCents: 300, DiscountCents: 500, FundedBy: enum.FundedByPlatform})
	got := map[enum.LedgerType]entry{}
	for _, e := range rows {
		got[e.kind] = e
	}
	if e := got[enum.LedgerTypeSale]; e.party != enum.LedgerPartyShop || e.amount != 7500 {
		t.Errorf("sale = %+v", e)
	}
	if e := got[enum.LedgerTypePlatformMargin]; e.party != enum.LedgerPartyPlatform || e.amount != 2500 {
		t.Errorf("margin = %+v", e)
	}
	if e := got[enum.LedgerTypeDiscount]; e.party != enum.LedgerPartyPlatform || e.amount != -500 {
		t.Errorf("discount = %+v", e)
	}
}

func TestPaidEntries_Refuses(t *testing.T) {
	for name, f := range map[string]PaidFacts{
		"margin above items":   {ItemsCents: 100, MarginCents: 101},
		"negative fee":         {ItemsCents: 100, ServiceFeeCents: -1},
		"discount above items": {ItemsCents: 100, DiscountCents: 101, FundedBy: enum.FundedByShop},
		"discount, no funder":  {ItemsCents: 100, DiscountCents: 10},
		"discount, bad funder": {ItemsCents: 100, DiscountCents: 10, FundedBy: "NOBODY"},
		"negative margin":      {ItemsCents: 100, MarginCents: -1},
		"negative discount":    {ItemsCents: 100, DiscountCents: -1, FundedBy: enum.FundedByShop},
	} {
		if _, err := paidEntries(f); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestDeliveredRefusesANegativeFee(t *testing.T) {
	if err := NewService().PostDelivered(t.Context(), nil, DeliveredFacts{DeliveryFeeCents: -1}); err == nil {
		t.Error("a negative delivery fee was accepted")
	}
	if err := NewService().PostDelivered(t.Context(), nil, DeliveredFacts{}); err != nil {
		t.Errorf("a free delivery: %v", err) // posts nothing, so it needs no transaction
	}
}
