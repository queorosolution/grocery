package order_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

// items 143500 (1 rice + 2 sugar), margin 3500, delivery fee 15000, no service fee
const (
	trioItems  = itemPrice + 2*sugarPrice
	trioTotal  = trioItems + deliveryFee
	trioMargin = (itemPrice - itemShop) + 2*(sugarPrice-sugarShop)
)

// awaiting is a trio the shop has approved: the customer can pay, and apply a coupon.
func (f fixture) awaiting() trio {
	f.a.T.Helper()
	x := f.trio()
	if w := f.review(f.staff("asha", "SHOP_STAFF"), x.o.ID, version(1, `"action":"APPROVE"`)); w.Code != http.StatusOK {
		f.a.T.Fatalf("approve = %d %s", w.Code, w.Body)
	}
	return x
}

// makeCoupon creates a coupon through the admin route. Defaults: 10 % off, paid by the shop, valid for a day.
func (f fixture) makeCoupon(admin apptest.Client, code string, extra ...any) {
	f.a.T.Helper()
	pairs := append([]any{"code", code, "type", "PERCENT", "value", 1000, "fundedBy", "SHOP", "shopId", f.shop.String(),
		"startsAt", time.Now().Add(-time.Hour).UTC().Format(time.RFC3339), "endsAt", time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)}, extra...)
	m := map[string]any{}
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i+1] == nil {
			delete(m, pairs[i].(string))
		} else {
			m[pairs[i].(string)] = pairs[i+1]
		}
	}
	raw, _ := json.Marshal(m)
	admin.OK(http.MethodPost, "/admin/coupons", string(raw))
}

func (f fixture) apply(c apptest.Client, id uuid.UUID, v int, code string) *httptest.ResponseRecorder {
	return c.Call(http.MethodPost, "/orders/"+id.String()+"/coupon", fmt.Sprintf(`{"version":%d,"code":%q}`, v, code))
}

func (f fixture) used(code string) int {
	return f.a.Count(`SELECT used_count FROM coupons WHERE code = $1`, code)
}

func TestCoupon_ApplyAndRemove(t *testing.T) {
	f := newFixture(t)
	x := f.awaiting()
	f.makeCoupon(f.a.AsAdmin(), "WELCOME10")

	f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/coupon", `{"version":2,"code":"WELCOME10","amount":5}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/coupon", `{"version":0,"code":"WELCOME10"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/coupon", `{"version":2}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/coupon", `{"version":9,"code":"WELCOME10"}`, http.StatusConflict, "STALE_ORDER")

	// The delivery rule changes after the bill was frozen: the order keeps its fee.
	f.a.Exec(`UPDATE delivery_fee_rules SET base_cents = 99999`)
	w := f.apply(f.me, x.o.ID, 2, " welcome10 ")
	if w.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", w.Code, w.Body)
	}
	got := apptest.Decode[order.Detail](t, w)
	discount := int64(trioItems / 10)
	if got.Version != 3 || got.Coupon == nil || got.Coupon.Code != "WELCOME10" || int64(got.Totals.DiscountCents) != discount ||
		int64(got.Totals.TotalCents) != trioTotal-discount || got.Totals.DeliveryFeeCents != deliveryFee || got.Totals.ItemsCents != trioItems {
		t.Fatalf("applied = %+v coupon %+v", got.Totals, got.Coupon)
	}
	if !hasAction(got, order.ActionRemoveCoupon) || hasAction(got, order.ActionApplyCoupon) {
		t.Errorf("actions = %v, want remove and not apply", got.Actions)
	}
	if f.used("WELCOME10") != 1 || f.a.Count(`SELECT count(*) FROM coupon_redemptions WHERE order_id = $1 AND amount_cents = $2 AND confirmed_at IS NULL`, x.o.ID, discount) != 1 ||
		f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND discount_funded_by = 'SHOP' AND coupon_id IS NOT NULL`, x.o.ID) != 1 {
		t.Error("the use, the reservation or the order's coupon fields are wrong")
	}
	if again := f.apply(f.me, x.o.ID, 3, "WELCOME10"); again.Code != http.StatusConflict || apptest.ErrorOf(t, again).Error.Code != "COUPON_ALREADY_APPLIED" {
		t.Errorf("a second coupon = %d %s, want 409 COUPON_ALREADY_APPLIED", again.Code, again.Body)
	}
	if f.used("WELCOME10") != 1 {
		t.Error("a refused second try took a use")
	}

	rm := f.me.Call(http.MethodDelete, "/orders/"+x.o.ID.String()+"/coupon", `{"version":2}`)
	if rm.Code != http.StatusConflict || apptest.ErrorOf(t, rm).Error.Code != "STALE_ORDER" {
		t.Errorf("remove with an old version = %d %s", rm.Code, rm.Body)
	}
	got = apptest.Decode[order.Detail](t, f.me.OK(http.MethodDelete, "/orders/"+x.o.ID.String()+"/coupon", `{"version":3}`))
	if got.Version != 4 || got.Coupon != nil || got.Totals.DiscountCents != 0 || int64(got.Totals.TotalCents) != trioTotal || !hasAction(got, order.ActionApplyCoupon) {
		t.Fatalf("removed = %+v coupon %+v actions %v", got.Totals, got.Coupon, got.Actions)
	}
	if f.used("WELCOME10") != 0 || f.a.Count(`SELECT count(*) FROM coupon_redemptions`) != 0 || f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND coupon_id IS NULL AND discount_funded_by IS NULL`, x.o.ID) != 1 {
		t.Error("removing did not give the use back")
	}
	f.me.Expect(http.MethodDelete, "/orders/"+x.o.ID.String()+"/coupon", `{"version":4}`, http.StatusBadRequest, "VALIDATION_ERROR")
	if f.apply(f.me, x.o.ID, 4, "WELCOME10").Code != http.StatusOK {
		t.Error("the coupon cannot be applied again after it was removed")
	}
}

func hasAction(d order.Detail, a order.Action) bool {
	for _, x := range d.Actions {
		if x == a {
			return true
		}
	}
	return false
}

// FR-CPN-02: a coupon goes on only while the order waits for payment, and only on the customer's own order.
func TestCoupon_OnlyWhenWaitingForPayment(t *testing.T) {
	f := newFixture(t)
	x := f.trio() // SUBMITTED
	f.makeCoupon(f.a.AsAdmin(), "WELCOME10")
	stranger := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "kamal", Email: "kamal@example.com", Name: "Kamal"}))

	if w := f.apply(f.me, x.o.ID, 1, "WELCOME10"); w.Code != http.StatusConflict || apptest.ErrorOf(t, w).Error.Code != "INVALID_TRANSITION" {
		t.Errorf("on a submitted order = %d %s", w.Code, w.Body)
	}
	f.a.Exec(`UPDATE orders SET status = 'AWAITING_PAYMENT', version = 2 WHERE id = $1`, x.o.ID)
	for name, c := range map[string]apptest.Client{"a stranger": stranger} {
		if w := f.apply(c, x.o.ID, 2, "WELCOME10"); w.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", name, w.Code)
		}
	}
	f.apply(stranger, uuid.New(), 2, "WELCOME10")
	if w := f.a.Call(http.MethodPost, "/v1/orders/"+x.o.ID.String()+"/coupon", `{"version":2,"code":"WELCOME10"}`); w.Code != http.StatusUnauthorized {
		t.Errorf("no login = %d, want 401", w.Code)
	}
	for _, status := range []enum.OrderStatus{enum.OrderStatusPaid, enum.OrderStatusCancelled, enum.OrderStatusExpired, enum.OrderStatusRevised} {
		f.a.Exec(`UPDATE orders SET status = $2 WHERE id = $1`, x.o.ID, status)
		if w := f.apply(f.me, x.o.ID, 2, "WELCOME10"); w.Code != http.StatusConflict {
			t.Errorf("on a %s order = %d, want 409", status, w.Code)
		}
		if w := f.me.Call(http.MethodDelete, "/orders/"+x.o.ID.String()+"/coupon", `{"version":2}`); w.Code != http.StatusConflict {
			t.Errorf("remove on a %s order = %d, want 409", status, w.Code)
		}
	}
	if f.used("WELCOME10") != 0 || f.a.Count(`SELECT count(*) FROM coupon_redemptions`) != 0 {
		t.Error("a refused coupon took a use")
	}
}

func TestCoupon_Refusals(t *testing.T) {
	f := newFixture(t)
	x := f.awaiting()
	admin := f.a.AsAdmin()
	f.makeCoupon(admin, "PAUSEDONE")
	f.makeCoupon(admin, "EXPIRED1")
	f.makeCoupon(admin, "LATER001")
	f.makeCoupon(admin, "BIGORDER", "minOrderCents", trioItems+1)
	f.makeCoupon(admin, "USEDUP01", "usageLimit", 1)
	f.makeCoupon(admin, "ONCEONLY")
	other, _ := f.readyShop("other")
	f.a.Exec(`INSERT INTO coupons (code, shop_id, type, value, starts_at, ends_at, funded_by, created_by)
		SELECT 'THEIRS01', $1, 'PERCENT', 1000, now() - interval '1 hour', now() + interval '1 day', 'SHOP', id FROM users LIMIT 1`, other)
	f.a.Exec(`UPDATE coupons SET status = 'PAUSED' WHERE code = 'PAUSEDONE'`)
	f.a.Exec(`UPDATE coupons SET starts_at = now() - interval '2 days', ends_at = now() - interval '1 day' WHERE code = 'EXPIRED1'`)
	f.a.Exec(`UPDATE coupons SET starts_at = now() + interval '1 hour', ends_at = now() + interval '2 days' WHERE code = 'LATER001'`)
	f.a.Exec(`UPDATE coupons SET used_count = 1 WHERE code = 'USEDUP01'`)

	for _, tc := range []struct {
		code   string
		status int
		err    string
	}{
		{"NOSUCHCODE", 400, "COUPON_INVALID"}, {"x", 400, "COUPON_INVALID"}, {"has space", 400, "COUPON_INVALID"},
		{"PAUSEDONE", 400, "COUPON_INVALID"}, {"THEIRS01", 400, "COUPON_INVALID"},
		{"EXPIRED1", 409, "COUPON_EXPIRED"}, {"LATER001", 409, "COUPON_EXPIRED"},
		{"BIGORDER", 409, "COUPON_MIN_ORDER"}, {"USEDUP01", 409, "COUPON_LIMIT"},
	} {
		w := f.apply(f.me, x.o.ID, 2, tc.code)
		if w.Code != tc.status || apptest.ErrorOf(t, w).Error.Code != tc.err {
			t.Errorf("%q = %d %s, want %d %s", tc.code, w.Code, w.Body, tc.status, tc.err)
		}
	}
	if f.a.Count(`SELECT sum(used_count) FROM coupons`) != 1 || f.a.Count(`SELECT count(*) FROM coupon_redemptions`) != 0 ||
		f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND version = 2 AND coupon_id IS NULL AND discount_cents = 0`, x.o.ID) != 1 {
		t.Fatal("a refused coupon took a use or changed the order")
	}

	f.a.Exec(`DELETE FROM rate_counters`) // the refusals above used up the hour's tries
	// One use per customer: the second order cannot use the coupon the first one holds.
	if f.apply(f.me, x.o.ID, 2, "ONCEONLY").Code != http.StatusOK {
		t.Fatal("the first use was refused")
	}
	second := f.secondOrder()
	if w := f.apply(f.me, second, 2, "ONCEONLY"); w.Code != http.StatusConflict || apptest.ErrorOf(t, w).Error.Code != "COUPON_LIMIT" {
		t.Errorf("second use by the same customer = %d %s, want 409 COUPON_LIMIT", w.Code, w.Body)
	}
	if f.used("ONCEONLY") != 1 {
		t.Errorf("used = %d, want 1", f.used("ONCEONLY"))
	}
	// Cancel the first order: its use comes back, and the second order can have it.
	f.me.OK(http.MethodPost, cancelPath(x.o.ID), `{"version":3}`)
	if f.used("ONCEONLY") != 0 || f.apply(f.me, second, 2, "ONCEONLY").Code != http.StatusOK {
		t.Error("the use did not come back with the cancel")
	}
}

// secondOrder is another approved order of the fixture's customer; it returns its id.
func (f fixture) secondOrder() uuid.UUID {
	f.a.T.Helper()
	rice := f.product(f.shop, "Red Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	o := f.placed(f.me, "second-order-key-1", f.body(`"slotId":"`+f.firstSlot(f.shop).String()+`"`, rice))
	if w := f.review(f.staff("bimal", "SHOP_OWNER"), o.ID, version(1, `"action":"APPROVE"`)); w.Code != http.StatusOK {
		f.a.T.Fatalf("approve = %d %s", w.Code, w.Body)
	}
	return o.ID
}

// FR-CPN-05: every try counts, a wrong code too.
func TestCoupon_TriesAreLimited(t *testing.T) {
	f := newFixture(t)
	x := f.awaiting()
	f.makeCoupon(f.a.AsAdmin(), "WELCOME10")
	for i := 1; i <= 10; i++ {
		if w := f.apply(f.me, x.o.ID, 2, fmt.Sprintf("WRONG%03d", i)); w.Code != http.StatusBadRequest {
			t.Fatalf("try %d = %d, want 400", i, w.Code)
		}
	}
	if w := f.apply(f.me, x.o.ID, 2, "WELCOME10"); w.Code != http.StatusTooManyRequests || apptest.ErrorOf(t, w).Error.Code != "RATE_LIMITED" {
		t.Errorf("try 11 = %d %s, want 429 RATE_LIMITED", w.Code, w.Body)
	}
	if f.used("WELCOME10") != 0 {
		t.Error("a limited try took a use")
	}
}

// FR-CPN-04 / BIZ-14: a platform-funded discount never takes more than the platform earns on the order.
func TestCoupon_PlatformFundedIsCapped(t *testing.T) {
	f := newFixture(t)
	x := f.awaiting()
	admin := f.a.AsAdmin()
	f.makeCoupon(admin, "PLATFORM1", "fundedBy", "PLATFORM", "type", "FIXED", "value", 20000, "shopId", nil)
	f.makeCoupon(admin, "LOSSOK01", "fundedBy", "PLATFORM", "type", "FIXED", "value", 20000, "shopId", nil, "allowLoss", true)

	got := decodeApplied(t, f.apply(f.me, x.o.ID, 2, "PLATFORM1"))
	if int64(got.Totals.DiscountCents) != trioMargin || int64(got.Totals.TotalCents) != trioTotal-trioMargin {
		t.Fatalf("capped discount = %+v, want the platform margin %d", got.Totals, trioMargin)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND discount_funded_by = 'PLATFORM'`, x.o.ID) != 1 {
		t.Error("the order does not say who pays the discount")
	}
	f.me.OK(http.MethodDelete, "/orders/"+x.o.ID.String()+"/coupon", `{"version":3}`)
	got = decodeApplied(t, f.apply(f.me, x.o.ID, 4, "LOSSOK01"))
	if got.Totals.DiscountCents != 20000 || int64(got.Totals.TotalCents) != trioTotal-20000 {
		t.Errorf("allow loss: %+v, want the whole 20000", got.Totals)
	}
}

// decodeApplied reads the order of a successful apply; anything else fails the test.
func decodeApplied(t *testing.T, w *httptest.ResponseRecorder) order.Detail {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", w.Code, w.Body)
	}
	return apptest.Decode[order.Detail](t, w)
}
