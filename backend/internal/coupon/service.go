package coupon

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/billing"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shared/validate"
)

type Service struct{ db *db.DB }

func NewService(d *db.DB) *Service { return &Service{db: d} }

// List returns coupons, newest first, at most 100. A bad status or shop id is a bad request.
func (s *Service) List(ctx context.Context, scope db.Scope, q ListQuery) ([]Coupon, error) {
	v := validate.New()
	if q.Status != "" {
		v.Check("status", enum.CouponStatus(q.Status).Valid(), "invalid")
	}
	var shopID *uuid.UUID
	if q.ShopID != "" {
		id, err := uuid.Parse(q.ShopID)
		v.Check("shopId", err == nil, "invalid")
		shopID = &id
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	var out []Coupon
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = list(ctx, tx, q.Status, shopID)
		return err
	})
	return out, err
}

// Create makes a coupon. The code is stored in upper case; one that exists is a
// validation error on the code, and so is a shop that does not exist.
func (s *Service) Create(ctx context.Context, scope db.Scope, in CreateInput) (Coupon, error) {
	in, err := in.check(clock.Now())
	if err != nil {
		return Coupon{}, err
	}
	var out Coupon
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if out, err = insert(ctx, tx, in, scope.UserID); err != nil {
			return createError(err)
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "coupon.create", Entity: "coupon", EntityID: out.ID.String(), After: out})
	})
	return out, err
}

func createError(err error) error {
	switch {
	case db.IsUniqueViolation(err):
		return apperr.Validation(map[string]string{"code": "taken"})
	case db.IsForeignKeyViolation(err):
		return apperr.Validation(map[string]string{"shopId": "unknown"})
	}
	return err
}

// Update changes the limits, dates or status of a coupon. The row is locked while
// the new values are checked against the uses already taken.
func (s *Service) Update(ctx context.Context, scope db.Scope, id uuid.UUID, p PatchInput) (Coupon, error) {
	var out Coupon
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := lockByID(ctx, tx, id)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		next, err := p.apply(before)
		if err != nil {
			return err
		}
		if out, err = save(ctx, tx, next); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "coupon.update", Entity: "coupon", EntityID: id.String(), Before: before, After: out})
	})
	return out, err
}

// ApplyInput names the order a coupon is for. The shop and the customer are the
// order's, read by the caller from the order it holds locked.
type ApplyInput struct {
	Code    string
	ShopID  uuid.UUID
	UserID  uuid.UUID
	OrderID uuid.UUID
}

// Pricer works out the discount of the reserved coupon, with the number of uses
// the customer already holds. billing.WithCoupon is the real one.
type Pricer func(c billing.CouponSnapshot, userUses int) (money.Cents, error)

// Applied is a coupon reserved for an order.
type Applied struct {
	CouponID      uuid.UUID
	Code          string
	FundedBy      enum.FundedBy
	DiscountCents money.Cents
}

// Apply reserves one use of the coupon for the order, inside the caller's
// transaction, so a refusal anywhere (the price, the order) gives the use back
// with the rest. An unknown, paused or other shop's coupon is COUPON_INVALID,
// all alike, so a code cannot be probed for another shop. A used-up coupon is
// COUPON_LIMIT; the dates, the customer's own limit and the minimum are checked
// by the pricer.
func (s *Service) Apply(ctx context.Context, tx *db.Tx, in ApplyInput, price Pricer) (Applied, error) {
	code, ok := Normalize(in.Code)
	if !ok {
		return Applied{}, apperr.New(apperr.CouponInvalid)
	}
	c, found, err := byCode(ctx, tx, code)
	if err != nil {
		return Applied{}, err
	}
	if !found || c.Status != enum.CouponStatusActive || (c.ShopID != nil && *c.ShopID != in.ShopID) {
		return Applied{}, apperr.New(apperr.CouponInvalid)
	}
	reserved, err := reserve(ctx, tx, c.ID)
	if err != nil {
		return Applied{}, err
	}
	if !reserved {
		return Applied{}, apperr.New(apperr.CouponLimit)
	}
	uses, err := userUses(ctx, tx, c.ID, in.UserID)
	if err != nil {
		return Applied{}, err
	}
	discount, err := price(snapshotOf(c), uses)
	if err != nil {
		return Applied{}, err
	}
	if err := insertRedemption(ctx, tx, c.ID, in.OrderID, in.UserID, int64(discount)); err != nil {
		if db.IsUniqueViolation(err) {
			return Applied{}, apperr.New(apperr.CouponAlreadyApplied)
		}
		return Applied{}, err
	}
	return Applied{CouponID: c.ID, Code: c.Code, FundedBy: c.FundedBy, DiscountCents: discount}, nil
}

func snapshotOf(c Coupon) billing.CouponSnapshot {
	return billing.CouponSnapshot{Type: c.Type, Value: c.Value, MaxDiscountCents: c.MaxDiscountCents, MinOrderCents: c.MinOrderCents,
		StartsAt: c.StartsAt, EndsAt: c.EndsAt, PerUserLimit: c.PerUserLimit, FundedBy: c.FundedBy, AllowLoss: c.AllowLoss}
}

// Release gives the order's reserved use back (T11, T12, remove). An order with
// no coupon, or whose use a payment confirmed, is left alone; the answer says
// whether a use came back.
func (s *Service) Release(ctx context.Context, tx *db.Tx, orderID uuid.UUID) (bool, error) {
	return release(ctx, tx, orderID)
}

// Confirm makes the order's use final (T10). It stays used from then on, also
// if the order is refunded (docs/03 section 11).
func (s *Service) Confirm(ctx context.Context, tx *db.Tx, orderID uuid.UUID) error {
	return confirm(ctx, tx, orderID, clock.Now())
}
