package order

import (
	"context"
	"math"
	"time"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/coupon"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shared/ratelimit"
	"gp/internal/shared/validate"
)

const (
	// couponTriesPerHour is FR-CPN-05: every try counts, also a wrong code.
	couponTriesPerHour = 10
	maxCouponCode      = 40
)

// CouponInput is the body of POST /orders/{id}/coupon.
type CouponInput struct {
	Version int    `json:"version"`
	Code    string `json:"code"`
}

// RemoveCouponInput is the body of DELETE /orders/{id}/coupon.
type RemoveCouponInput struct {
	Version int `json:"version"`
}

// frozenBill is the money of an order as it was frozen, and the coupon on it.
type frozenBill struct {
	totals   billing.Totals
	margin   money.Cents
	couponID *uuid.UUID
}

// ApplyCoupon puts a coupon on an order that waits for payment (FR-CPN-02). The
// use is reserved and the discount worked out in one transaction: a refusal
// anywhere gives the use back. Prices, fees and the platform margin stay as they
// were frozen; only the discount and the total move.
func (s *Service) ApplyCoupon(ctx context.Context, scope db.Scope, orderID uuid.UUID, in CouponInput) (Detail, error) {
	if err := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Str("code", in.Code, 1, maxCouponCode).Err(); err != nil {
		return Detail{}, err
	}
	if err := ratelimit.Hit(ctx, s.db, "coupon:"+scope.UserID.String(), couponTriesPerHour, time.Hour); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		o, bill, err := s.lockForCoupon(ctx, tx, scope, orderID, in.Version)
		if err != nil {
			return err
		}
		if bill.couponID != nil {
			return apperr.New(apperr.CouponAlreadyApplied)
		}
		var totals billing.Totals
		applied, err := s.coupons.Apply(ctx, tx, coupon.ApplyInput{Code: in.Code, ShopID: o.ShopID, UserID: o.CustomerID, OrderID: o.ID},
			func(c billing.CouponSnapshot, uses int) (money.Cents, error) {
				var err error
				totals, err = billing.WithCoupon(bill.totals, bill.margin, &c, uses, clock.Now())
				return totals.DiscountCents, err
			})
		if err != nil {
			return err
		}
		if err := saveCoupon(ctx, tx, o, &applied.CouponID, &applied.FundedBy, totals); err != nil {
			return err
		}
		out, err = s.detail(ctx, tx, scope, orderID)
		return err
	})
	return out, err
}

// RemoveCoupon takes the coupon off and gives its use back.
func (s *Service) RemoveCoupon(ctx context.Context, scope db.Scope, orderID uuid.UUID, in RemoveCouponInput) (Detail, error) {
	if err := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Err(); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		o, bill, err := s.lockForCoupon(ctx, tx, scope, orderID, in.Version)
		if err != nil {
			return err
		}
		if bill.couponID == nil {
			return apperr.Validation(map[string]string{"coupon": "not_applied"})
		}
		if _, err := s.coupons.Release(ctx, tx, o.ID); err != nil {
			return err
		}
		totals, err := billing.WithCoupon(bill.totals, bill.margin, nil, 0, clock.Now())
		if err != nil {
			return err
		}
		if err := saveCoupon(ctx, tx, o, nil, nil, totals); err != nil {
			return err
		}
		out, err = s.detail(ctx, tx, scope, orderID)
		return err
	})
	return out, err
}

// lockForCoupon locks the order and checks what both coupon calls need: it is the
// caller's own, the version is the one they saw, and it waits for payment.
func (s *Service) lockForCoupon(ctx context.Context, tx *db.Tx, scope db.Scope, id uuid.UUID, version int) (orderRow, frozenBill, error) {
	o, found, err := lockOrder(ctx, tx, id)
	switch {
	case err != nil || !found || !inScope(scope, o):
		return o, frozenBill{}, apperr.NotFoundUnless(err, found && inScope(scope, o))
	case o.Version != version:
		return o, frozenBill{}, apperr.New(apperr.StaleOrder)
	case o.Status != enum.OrderStatusAwaitingPayment:
		return o, frozenBill{}, apperr.New(apperr.InvalidTransition)
	}
	var b frozenBill
	err = tx.QueryRow(ctx, `
		SELECT items_cents, delivery_fee_cents, service_fee_cents, platform_margin_cents, coupon_id FROM orders WHERE id = $1`, id).
		Scan(&b.totals.ItemsCents, &b.totals.DeliveryFeeCents, &b.totals.ServiceFeeCents, &b.margin, &b.couponID)
	return o, b, err
}

// saveCoupon writes the coupon and the new money on the order. The version moves
// on, so a screen that still shows the old total gets STALE_ORDER instead of paying it.
func saveCoupon(ctx context.Context, tx *db.Tx, o orderRow, couponID *uuid.UUID, fundedBy *enum.FundedBy, t billing.Totals) error {
	tag, err := tx.Exec(ctx, `
		UPDATE orders SET coupon_id = $3, discount_funded_by = $4, discount_cents = $5, total_cents = $6, version = version + 1
		WHERE id = $1 AND version = $2`, o.ID, o.Version, couponID, fundedBy, t.DiscountCents, t.TotalCents)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperr.New(apperr.StaleOrder)
	}
	return nil
}
