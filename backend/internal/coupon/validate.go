package coupon

import (
	"time"

	"gp/internal/shared/apperr"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// check validates a new coupon and returns it with the code normalised.
func (in CreateInput) check(now time.Time) (CreateInput, error) {
	code, ok := Normalize(in.Code)
	in.Code = code
	if in.PerUserLimit == 0 {
		in.PerUserLimit = 1
	}
	v := validate.New().
		Check("code", ok, "invalid").
		Check("type", in.Type.Valid(), "invalid").
		Check("fundedBy", in.FundedBy.Valid(), "invalid").
		Check("startsAt", !in.StartsAt.IsZero(), "required").
		Check("endsAt", !in.EndsAt.IsZero(), "required").
		Check("endsAt", in.EndsAt.After(in.StartsAt), "before_start").
		Check("endsAt", in.EndsAt.After(now), "in_past").
		Check("minOrderCents", in.MinOrderCents >= 0 && int64(in.MinOrderCents) <= maxMoney, "out_of_range").
		Check("perUserLimit", in.PerUserLimit >= 1 && in.PerUserLimit <= maxPerUser, "out_of_range").
		Check("shopId", in.FundedBy != enum.FundedByShop || in.ShopID != nil, "required_when_shop_funded").
		Check("allowLoss", !in.AllowLoss || in.FundedBy == enum.FundedByPlatform, "platform_only")
	switch in.Type {
	case enum.CouponTypePercent:
		v.Check("value", in.Value >= 1 && in.Value <= maxPercentBP, "out_of_range")
	case enum.CouponTypeFixed:
		v.Check("value", in.Value >= 1 && in.Value <= maxMoney, "out_of_range")
	}
	if in.MaxDiscountCents != nil {
		v.Check("maxDiscountCents", *in.MaxDiscountCents >= 1 && int64(*in.MaxDiscountCents) <= maxMoney, "out_of_range")
	}
	if in.UsageLimit != nil {
		v.Check("usageLimit", *in.UsageLimit >= 1 && *in.UsageLimit <= maxUses, "out_of_range")
	}
	return in, v.Err()
}

// apply merges the patch into the coupon and validates the result as a whole.
func (p PatchInput) apply(c Coupon) (Coupon, error) {
	if p.Status.Set {
		c.Status = p.Status.Value
	}
	if p.StartsAt.Set {
		c.StartsAt = p.StartsAt.Value
	}
	if p.EndsAt.Set {
		c.EndsAt = p.EndsAt.Value
	}
	if p.PerUserLimit.Set {
		c.PerUserLimit = p.PerUserLimit.Value
	}
	if p.MinOrderCents.Set {
		c.MinOrderCents = p.MinOrderCents.Value
	}
	if p.AllowLoss.Set {
		c.AllowLoss = p.AllowLoss.Value
	}
	if p.UsageLimit.Set {
		c.UsageLimit = p.UsageLimit.Ptr()
	}
	if p.MaxDiscountCents.Set {
		c.MaxDiscountCents = p.MaxDiscountCents.Ptr()
	}
	v := validate.New().
		Check("status", c.Status.Valid() && !p.Status.Null, "invalid").
		Check("startsAt", !c.StartsAt.IsZero() && !p.StartsAt.Null, "required").
		Check("endsAt", !c.EndsAt.IsZero() && !p.EndsAt.Null && c.EndsAt.After(c.StartsAt), "before_start").
		Check("perUserLimit", !p.PerUserLimit.Null && c.PerUserLimit >= 1 && c.PerUserLimit <= maxPerUser, "out_of_range").
		Check("minOrderCents", !p.MinOrderCents.Null && c.MinOrderCents >= 0 && int64(c.MinOrderCents) <= maxMoney, "out_of_range").
		Check("allowLoss", !p.AllowLoss.Null && (!c.AllowLoss || c.FundedBy == enum.FundedByPlatform), "platform_only")
	if c.UsageLimit != nil {
		v.Check("usageLimit", *c.UsageLimit >= 1 && *c.UsageLimit <= maxUses, "out_of_range").
			Check("usageLimit", *c.UsageLimit >= c.UsedCount, "below_used")
	}
	if c.MaxDiscountCents != nil {
		v.Check("maxDiscountCents", *c.MaxDiscountCents >= 1 && int64(*c.MaxDiscountCents) <= maxMoney, "out_of_range")
	}
	if !p.Status.Set && !p.StartsAt.Set && !p.EndsAt.Set && !p.UsageLimit.Set && !p.PerUserLimit.Set &&
		!p.MaxDiscountCents.Set && !p.MinOrderCents.Set && !p.AllowLoss.Set {
		return c, apperr.Validation(map[string]string{"body": "empty"})
	}
	return c, v.Err()
}
