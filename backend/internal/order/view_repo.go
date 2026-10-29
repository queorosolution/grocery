package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shop"
)

// detail loads the order and maps it for the caller's viewer. An order the
// caller cannot see is NOT_FOUND.
func (s *Service) detail(ctx context.Context, tx *db.Tx, scope db.Scope, id uuid.UUID) (Detail, error) {
	v, ok := viewerOf(scope)
	if !ok {
		return Detail{}, apperr.New(apperr.Forbidden)
	}
	d, found, err := loadData(ctx, tx, id)
	if err != nil || !found || !inScope(scope, d.orderRow) {
		return Detail{}, apperr.NotFoundUnless(err, found && inScope(scope, d.orderRow))
	}
	if v == ViewCustomer && d.Status == enum.OrderStatusOutForDelivery {
		code, _, ok, err := s.codeOf(ctx, tx, d.ID)
		if err != nil {
			return Detail{}, err
		}
		if ok {
			d.DeliveryCode = &code
		}
	}
	if v != ViewDriver {
		t, err := s.shops.TimeoutsOf(ctx, tx, d.ShopID)
		if err != nil {
			return Detail{}, err
		}
		d.ExpiresAt = expiry(d, t)
	}
	return d.Detail(v, scope), nil
}

// expiry is when the wait the order is in runs out: the time it entered the
// status plus that wait's timeout. Statuses that nobody waits on have none.
func expiry(d orderData, t shop.Timeouts) *time.Time {
	var wait time.Duration
	switch d.Status {
	case enum.OrderStatusSubmitted:
		wait = t.Review
	case enum.OrderStatusRevised:
		wait = t.Revision
	case enum.OrderStatusAwaitingPayment:
		wait = t.Payment
	default:
		return nil
	}
	for i := len(d.Steps) - 1; i >= 0; i-- {
		if d.Steps[i].Status == d.Status {
			at := d.Steps[i].At.Add(wait)
			return &at
		}
	}
	return nil
}

func loadData(ctx context.Context, tx *db.Tx, id uuid.UUID) (orderData, bool, error) {
	var d orderData
	var couponCode, payStatus *string
	var starts, ends *time.Time
	err := tx.QueryRow(ctx, `
		SELECT o.id, o.code, o.shop_id, o.customer_id, o.driver_id, o.status, o.version, o.revision_no, o.slot_id,
		       o.address, o.lat, o.lng, o.items_cents, o.delivery_fee_cents, o.service_fee_cents, o.discount_cents, o.total_cents,
		       o.payment_provider, o.paid_at, sh.slug, sh.name, sh.phone, sl.starts_at, sl.ends_at, c.code, u.name,
		       (SELECT p.status FROM payments p WHERE p.order_id = o.id ORDER BY p.created_at DESC, p.id DESC LIMIT 1)
		FROM orders o
		JOIN shops sh ON sh.id = o.shop_id
		JOIN users u ON u.id = o.customer_id
		LEFT JOIN slots sl ON sl.id = o.slot_id
		LEFT JOIN coupons c ON c.id = o.coupon_id
		WHERE o.id = $1`, id).
		Scan(&d.ID, &d.Code, &d.ShopID, &d.CustomerID, &d.DriverID, &d.Status, &d.Version, &d.RevisionNo, &d.SlotID,
			&d.Address, &d.Lat, &d.Lng, &d.Totals.ItemsCents, &d.Totals.DeliveryFeeCents, &d.Totals.ServiceFeeCents,
			&d.Totals.DiscountCents, &d.Totals.TotalCents, &d.Payment.Provider, &d.PaidAt,
			&d.Shop.Slug, &d.Shop.Name, &d.Shop.Phone, &starts, &ends, &couponCode, &d.CustomerNm, &payStatus)
	if err != nil {
		return d, false, db.IgnoreNoRows(err)
	}
	d.Shop.ID, d.Payment.Status = d.ShopID, payStatus
	if starts != nil && ends != nil {
		d.Slot = &SlotRef{StartsAt: *starts, EndsAt: *ends}
	}
	if couponCode != nil {
		d.Coupon = &CouponRef{Code: *couponCode, DiscountCents: d.Totals.DiscountCents}
	}
	if d.Items, err = loadItems(ctx, tx, id); err != nil {
		return d, false, err
	}
	d.Steps, err = loadSteps(ctx, tx, id)
	return d, true, err
}

func loadItems(ctx context.Context, tx *db.Tx, orderID uuid.UUID) ([]LineView, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, name, unit, qty::int, online_price_cents, line_cents, state, sub_pref, revision_no, replaces_item_id
		FROM order_items WHERE order_id = $1 ORDER BY revision_no, name, id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LineView{}
	for rows.Next() {
		var l LineView
		var price, line money.Cents
		if err := rows.Scan(&l.ID, &l.Name, &l.Unit, &l.Qty, &price, &line, &l.State, &l.SubPref, &l.RevisionNo, &l.ReplacesItemID); err != nil {
			return nil, err
		}
		l.UnitPriceCents, l.LineCents = &price, &line
		out = append(out, l)
	}
	return out, rows.Err()
}

func loadSteps(ctx context.Context, tx *db.Tx, orderID uuid.UUID) ([]Step, error) {
	rows, err := tx.Query(ctx, `SELECT to_status, at FROM order_events WHERE order_id = $1 ORDER BY at, id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Step{}
	for rows.Next() {
		var st Step
		if err := rows.Scan(&st.Status, &st.At); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Get is the order detail for the caller.
func (s *Service) Get(ctx context.Context, scope db.Scope, id uuid.UUID) (Detail, error) {
	var out Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = s.detail(ctx, tx, scope, id)
		return err
	})
	return out, err
}
