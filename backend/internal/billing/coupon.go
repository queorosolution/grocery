package billing

import (
	"time"

	"gp/internal/shared/apperr"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// discount works out the coupon's discount on the items total. It refuses a
// coupon outside its dates, one the customer has used up, and one whose minimum
// order is not met. The result never exceeds the coupon's own cap, the items
// total, or (for a platform-funded coupon without allow_loss) the platform's
// income from the order, so the platform cannot be made to pay for a sale
// (FR-CPN-04). A shop-funded coupon comes out of the shop's share, so the income
// cap does not apply to it.
func discount(c *CouponSnapshot, uses int, now time.Time, items, platformIncome money.Cents) (money.Cents, error) {
	switch {
	case now.Before(c.StartsAt) || !now.Before(c.EndsAt):
		return 0, apperr.New(apperr.CouponExpired)
	case c.PerUserLimit > 0 && uses >= c.PerUserLimit:
		return 0, apperr.New(apperr.CouponLimit)
	case items < c.MinOrderCents:
		return 0, apperr.New(apperr.CouponMinOrder)
	}
	var d money.Cents
	switch c.Type {
	case enum.CouponTypePercent:
		var err error
		if d, err = money.ApplyBP(items, int(c.Value)); err != nil {
			return 0, err
		}
	case enum.CouponTypeFixed:
		d = money.Cents(c.Value)
	default:
		return 0, apperr.New(apperr.CouponInvalid)
	}
	if c.MaxDiscountCents != nil {
		d = min(d, *c.MaxDiscountCents)
	}
	d = min(d, items)
	if c.FundedBy == enum.FundedByPlatform && !c.AllowLoss {
		d = min(d, max(0, platformIncome))
	}
	return max(d, 0), nil
}

// WithCoupon puts a coupon on an order whose bill is already frozen, or takes it
// off (c == nil). The items, delivery fee and service fee stay as they were, even
// if the shop's rules changed since: only the discount and the total move. margin
// is the platform margin stored with the order. Any discount already in frozen is
// replaced, not added to.
func WithCoupon(frozen Totals, margin money.Cents, c *CouponSnapshot, uses int, now time.Time) (Totals, error) {
	t := Totals{ItemsCents: frozen.ItemsCents, DeliveryFeeCents: frozen.DeliveryFeeCents, ServiceFeeCents: frozen.ServiceFeeCents}
	if c != nil {
		income, err := money.Add(margin, t.ServiceFeeCents)
		if err != nil {
			return Totals{}, err
		}
		if t.DiscountCents, err = discount(c, uses, now, t.ItemsCents, income); err != nil {
			return Totals{}, err
		}
	}
	total, err := money.Add(t.ItemsCents, t.DeliveryFeeCents)
	if err == nil {
		total, err = money.Add(total, t.ServiceFeeCents)
	}
	if err != nil {
		return Totals{}, err
	}
	t.TotalCents = total - t.DiscountCents
	return t, nil
}
