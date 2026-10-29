package billing

import (
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
func discount(in BillInput, items, platformIncome money.Cents) (money.Cents, error) {
	c := in.Coupon
	switch {
	case in.Now.Before(c.StartsAt) || !in.Now.Before(c.EndsAt):
		return 0, apperr.New(apperr.CouponExpired)
	case c.PerUserLimit > 0 && in.UserCouponUses >= c.PerUserLimit:
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
