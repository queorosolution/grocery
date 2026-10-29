package billing

import (
	"testing"
	"time"

	"gp/internal/shared/apperr"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

func coupon(c CouponSnapshot) BillInput {
	if c.StartsAt.IsZero() {
		c.StartsAt, c.EndsAt = now.Add(-time.Hour), now.Add(time.Hour)
	}
	if c.PerUserLimit == 0 {
		c.PerUserLimit = 1
	}
	in := input(line(rice, 1), line(sugar, 2)) // items 205500, margin 5500
	in.Coupon = &c
	return in
}

func TestBuild_CouponDiscount(t *testing.T) {
	capAt := money.Cents(10000)
	for name, tc := range map[string]struct {
		c       CouponSnapshot
		service money.Cents // the shop's minimum margin, to make a service fee
		want    money.Cents
	}{
		"percent":                            {CouponSnapshot{Type: enum.CouponTypePercent, Value: 1000, FundedBy: enum.FundedByShop}, 0, 20550},
		"percent rounds":                     {CouponSnapshot{Type: enum.CouponTypePercent, Value: 333, FundedBy: enum.FundedByShop}, 0, 6843}, // 205500 x 3.33% = 6843.15
		"fixed":                              {CouponSnapshot{Type: enum.CouponTypeFixed, Value: 5000, FundedBy: enum.FundedByShop}, 0, 5000},
		"capped by max":                      {CouponSnapshot{Type: enum.CouponTypePercent, Value: 5000, MaxDiscountCents: &capAt, FundedBy: enum.FundedByShop}, 0, 10000},
		"never above the items":              {CouponSnapshot{Type: enum.CouponTypeFixed, Value: 999999999, FundedBy: enum.FundedByShop}, 0, 205500},
		"platform: capped by income":         {CouponSnapshot{Type: enum.CouponTypeFixed, Value: 20000, FundedBy: enum.FundedByPlatform}, 0, 5500},
		"platform: income incl. service fee": {CouponSnapshot{Type: enum.CouponTypeFixed, Value: 20000, FundedBy: enum.FundedByPlatform}, 10000, 10000},
		"platform: allow loss":               {CouponSnapshot{Type: enum.CouponTypeFixed, Value: 20000, FundedBy: enum.FundedByPlatform, AllowLoss: true}, 0, 20000},
		"shop funded ignores the income cap": {CouponSnapshot{Type: enum.CouponTypeFixed, Value: 20000, FundedBy: enum.FundedByShop}, 0, 20000},
	} {
		in := coupon(tc.c)
		in.Shop.MinPlatformMarginCents = tc.service
		got, err := Build(in)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if got.Totals.DiscountCents != tc.want {
			t.Errorf("%s: discount %d, want %d", name, got.Totals.DiscountCents, tc.want)
		}
		sum := got.Totals.ItemsCents + got.Totals.DeliveryFeeCents + got.Totals.ServiceFeeCents - got.Totals.DiscountCents
		if got.Totals.TotalCents != sum || got.Totals.TotalCents < 0 {
			t.Errorf("%s: total %d, want %d", name, got.Totals.TotalCents, sum)
		}
	}
}

func TestBuild_CouponRefusals(t *testing.T) {
	base := CouponSnapshot{Type: enum.CouponTypeFixed, Value: 1000, FundedBy: enum.FundedByShop}
	for name, tc := range map[string]struct {
		edit func(*BillInput)
		want apperr.Code
	}{
		"before it starts":  {func(in *BillInput) { in.Coupon.StartsAt, in.Coupon.EndsAt = now.Add(time.Minute), now.Add(time.Hour) }, apperr.CouponExpired},
		"at its end":        {func(in *BillInput) { in.Coupon.EndsAt = now }, apperr.CouponExpired},
		"used up":           {func(in *BillInput) { in.UserCouponUses = 1 }, apperr.CouponLimit},
		"below its minimum": {func(in *BillInput) { in.Coupon.MinOrderCents = 205501 }, apperr.CouponMinOrder},
		"unknown type":      {func(in *BillInput) { in.Coupon.Type = "BOGUS" }, apperr.CouponInvalid},
	} {
		in := coupon(base)
		tc.edit(&in)
		if _, err := Build(in); code(err) != tc.want {
			t.Errorf("%s: error = %v, want %s", name, err, tc.want)
		}
	}
	in := coupon(base)
	in.Coupon.MinOrderCents = 205500 // exactly met
	if _, err := Build(in); err != nil {
		t.Errorf("minimum exactly met: %v", err)
	}
	in = coupon(base)
	in.UserCouponUses = 0
	in.Coupon.PerUserLimit = 3
	in.UserCouponUses = 2
	if _, err := Build(in); err != nil {
		t.Errorf("third use of three: %v", err)
	}
}

// No coupon, no discount; and a coupon on an empty bill does nothing.
func TestBuild_CouponNeedsLines(t *testing.T) {
	in := input(line(soap, 1))
	in.Coupon = &CouponSnapshot{Type: enum.CouponTypeFixed, Value: 1000, FundedBy: enum.FundedByShop, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), PerUserLimit: 1}
	got, err := Build(in)
	if err != nil || got.Totals.DiscountCents != 0 {
		t.Errorf("empty bill with a coupon: %+v %v", got.Totals, err)
	}
	if got, _ = Build(input(line(rice, 1))); got.Totals.DiscountCents != 0 {
		t.Error("a discount without a coupon")
	}
}

// WithCoupon works on a frozen bill: the fees stay, only the discount and the total move.
func TestWithCoupon(t *testing.T) {
	frozen := Totals{ItemsCents: 205500, DeliveryFeeCents: 15000, ServiceFeeCents: 2000, DiscountCents: 999, TotalCents: 1}
	margin := money.Cents(5500)
	at := func(c CouponSnapshot) *CouponSnapshot {
		c.StartsAt, c.EndsAt, c.PerUserLimit = now.Add(-time.Hour), now.Add(time.Hour), 1
		return &c
	}
	for name, tc := range map[string]struct {
		c    *CouponSnapshot
		want Totals
	}{
		"removed":         {nil, Totals{ItemsCents: 205500, DeliveryFeeCents: 15000, ServiceFeeCents: 2000, TotalCents: 222500}},
		"percent":         {at(CouponSnapshot{Type: enum.CouponTypePercent, Value: 1000, FundedBy: enum.FundedByShop}), Totals{ItemsCents: 205500, DeliveryFeeCents: 15000, ServiceFeeCents: 2000, DiscountCents: 20550, TotalCents: 201950}},
		"platform cap":    {at(CouponSnapshot{Type: enum.CouponTypeFixed, Value: 20000, FundedBy: enum.FundedByPlatform}), Totals{ItemsCents: 205500, DeliveryFeeCents: 15000, ServiceFeeCents: 2000, DiscountCents: 7500, TotalCents: 215000}}, // income = margin 5500 + fee 2000
		"allow loss":      {at(CouponSnapshot{Type: enum.CouponTypeFixed, Value: 20000, FundedBy: enum.FundedByPlatform, AllowLoss: true}), Totals{ItemsCents: 205500, DeliveryFeeCents: 15000, ServiceFeeCents: 2000, DiscountCents: 20000, TotalCents: 202500}},
		"capped by items": {at(CouponSnapshot{Type: enum.CouponTypeFixed, Value: 999999999, FundedBy: enum.FundedByShop}), Totals{ItemsCents: 205500, DeliveryFeeCents: 15000, ServiceFeeCents: 2000, TotalCents: 17000, DiscountCents: 205500}},
	} {
		got, err := WithCoupon(frozen, margin, tc.c, 0, now)
		if err != nil || got != tc.want {
			t.Errorf("%s: %+v, %v; want %+v", name, got, err, tc.want)
		}
	}
	for name, tc := range map[string]struct {
		c    *CouponSnapshot
		uses int
		want apperr.Code
	}{
		"not started": {&CouponSnapshot{Type: enum.CouponTypeFixed, Value: 1, StartsAt: now.Add(time.Minute), EndsAt: now.Add(time.Hour), PerUserLimit: 1}, 0, apperr.CouponExpired},
		"used up":     {at(CouponSnapshot{Type: enum.CouponTypeFixed, Value: 1}), 1, apperr.CouponLimit},
		"minimum":     {at(CouponSnapshot{Type: enum.CouponTypeFixed, Value: 1, MinOrderCents: 205501}), 0, apperr.CouponMinOrder},
	} {
		if _, err := WithCoupon(frozen, margin, tc.c, tc.uses, now); code(err) != tc.want {
			t.Errorf("%s: %v, want %s", name, err, tc.want)
		}
	}
	// Same facts, same answer as Build.
	c := at(CouponSnapshot{Type: enum.CouponTypePercent, Value: 1000, FundedBy: enum.FundedByShop})
	in := input(line(rice, 1), line(sugar, 2))
	in.Coupon = c
	built, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := WithCoupon(built.Totals, built.MarginCents, c, 0, now); err != nil || got != built.Totals {
		t.Errorf("WithCoupon = %+v, %v; Build gave %+v", got, err, built.Totals)
	}
}
