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
