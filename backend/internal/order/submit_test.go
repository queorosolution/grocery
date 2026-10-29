package order_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gp/internal/order"
	"gp/internal/shared/enum"
)

const key1 = "key-0000000000001"

// FR-BILL-02, T01: the order is written, the slot place taken, the events and the review timer are in the outbox, the cart is gone.
func TestSubmit_CreatesTheOrder(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 2, 1}) // the cart claims Rs 0.01
	slot := f.firstSlot(f.shop)

	o := f.placed(f.me, key1, f.body(`"slotId":"`+slot.String()+`","note":"  ring the bell  "`, rice))
	if o.Status != enum.OrderStatusSubmitted || o.Version != 1 || !strings.HasPrefix(o.Code, "KR-") || len(o.Code) != 9 {
		t.Fatalf("order = %+v", o)
	}
	if len(o.Lines) != 1 || o.Lines[0].Qty != 2 || *o.Lines[0].UnitPriceCents != itemPrice {
		t.Errorf("lines = %+v, want 2 at the database price", o.Lines)
	}
	if o.Totals.ItemsCents != 2*itemPrice || o.Totals.DeliveryFeeCents != deliveryFee || o.Totals.TotalCents != 2*itemPrice+deliveryFee {
		t.Errorf("totals = %+v", o.Totals)
	}
	if o.Slot == nil || o.Shop.Slug != "krishna" || o.Address.Line1 == "" || o.Payment.Provider != enum.PaymentProviderManual {
		t.Errorf("order parts = slot %v shop %+v address %+v payment %+v", o.Slot, o.Shop, o.Address, o.Payment)
	}
	if len(o.Actions) != 1 || o.Actions[0] != order.ActionCancel {
		t.Errorf("actions = %v, want only CANCEL", o.Actions)
	}
	if len(o.Timeline) != 1 || o.ExpiresAt == nil || o.ExpiresAt.Sub(o.Timeline[0].At) != 15*time.Minute {
		t.Errorf("timeline %+v expires %v, want the 15 minute review window", o.Timeline, o.ExpiresAt)
	}

	count := func(q string, args ...any) int { return f.a.Count(q, args...) }
	if count(`SELECT count(*) FROM orders WHERE status = 'SUBMITTED' AND total_cents = $1 AND note = 'ring the bell' AND cart_ref = $2`, o.Totals.TotalCents, cartID) != 1 ||
		count(`SELECT count(*) FROM order_items WHERE order_id = $1 AND online_price_cents = $2 AND shop_price_cents = $3`, o.ID, itemPrice, itemShop) != 1 {
		t.Error("the order or its line is not saved as priced")
	}
	if count(`SELECT booked FROM slots WHERE id = $1`, slot) != 1 {
		t.Error("the slot place was not taken")
	}
	if count(`SELECT count(*) FROM order_events WHERE order_id = $1 AND from_status IS NULL AND to_status = 'SUBMITTED' AND actor_role = 'CUSTOMER'`, o.ID) != 1 {
		t.Error("the history row is missing")
	}
	if count(`SELECT count(*) FROM outbox WHERE type = 'order.submitted' AND entity_id = $1`, o.ID.String()) != 1 ||
		count(`SELECT count(*) FROM outbox WHERE type = 'timer.schedule' AND payload->>'kind' = 'review' AND entity_id = $1`, o.ID.String()) != 1 {
		t.Error("the order.submitted event or the review timer is not in the outbox")
	}
	if f.a.Cart.HasCart(f.tenant, cartID) {
		t.Error("the cart was not deleted after the order was saved")
	}

	// The address is a snapshot: later edits do not change the order.
	f.a.Exec(`UPDATE addresses SET line1 = 'Somewhere else entirely'`)
	if got := f.get(f.me, o.ID); got.Address.Line1 != o.Address.Line1 {
		t.Errorf("the order address changed to %q", got.Address.Line1)
	}
}

// Every refusal leaves nothing behind: no order, no place taken, the cart kept, and the key free to use again.
func TestSubmit_Refusals(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	tea := f.product(f.shop, "Tea 400g", 50000, 51000, false)
	slot := f.firstSlot(f.shop)
	withSlot := `"slotId":"` + slot.String() + `"`
	f.slot(f.shop, 10) // a second window, so the shop stays ready when the first one is switched off
	set := func(sql string, args ...any) { t.Helper(); f.a.Exec(sql, args...) }

	cases := []struct {
		name    string
		arrange func()
		undo    func()
		body    string
		status  int
		code    string
	}{
		{"no slot for a slot shop", nil, nil, f.body("", rice), 400, "VALIDATION_ERROR"},
		{"phone not verified", func() { set(`UPDATE users SET phone_verified_at = NULL`) }, func() { f.verify("nimal@example.com") }, f.body(withSlot, rice), 409, "PHONE_NOT_VERIFIED"},
		{"shop closed", func() { set(`UPDATE shops SET accepting_orders = false`) }, func() { set(`UPDATE shops SET accepting_orders = true`) }, f.body(withSlot, rice), 409, "SHOP_CLOSED"},
		{"shop not ready", func() { set(`UPDATE payment_accounts SET status = 'DISABLED'`) }, func() { set(`UPDATE payment_accounts SET status = 'ACTIVE'`) }, f.body(withSlot, rice), 409, "SHOP_NOT_READY"},
		{"below the minimum", func() { set(`UPDATE shop_settings SET min_order_cents = 9999999`) }, func() { set(`UPDATE shop_settings SET min_order_cents = 0`) }, f.body(withSlot, rice), 409, "BELOW_MIN_ORDER"},
		{"out of range", func() { set(`UPDATE delivery_fee_rules SET max_km = 0.5`) }, func() { set(`UPDATE delivery_fee_rules SET max_km = 8`) }, f.body(withSlot, rice), 409, "OUT_OF_RANGE"},
		{"a line that is not for sale", func() { f.cart(f.tenant, cartLine{rice, 1, itemPrice}, cartLine{tea, 1, 51000}) }, nil, f.body(withSlot, rice), 409, "ITEM_UNAVAILABLE"},
		{"too many of one product", func() { f.cart(f.tenant, cartLine{rice, 21, itemPrice}) }, nil, f.body(withSlot, rice), 409, "QTY_LIMIT"},
		{"a slot of nobody", func() { f.cart(f.tenant, cartLine{rice, 1, itemPrice}) }, nil, f.body(`"slotId":"00000000-0000-4000-8000-000000000001"`, rice), 404, "NOT_FOUND"},
		{"a closed slot", func() { set(`UPDATE slots SET status = 'CLOSED' WHERE id = $1`, slot) }, func() { set(`UPDATE slots SET status = 'OPEN' WHERE id = $1`, slot) }, f.body(withSlot, rice), 409, "SLOT_CLOSED"},
		{"a full slot", func() { set(`UPDATE slots SET capacity = 0 WHERE id = $1`, slot) }, func() { set(`UPDATE slots SET capacity = 10 WHERE id = $1`, slot) }, f.body(withSlot, rice), 409, "SLOT_FULL"},
		{"a missing cart", nil, nil, strings.Replace(f.body(withSlot, rice), cartID, "crt_missing", 1), 409, "CART_WRONG_SHOP"},
	}
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	for i, c := range cases {
		if c.arrange != nil {
			c.arrange()
		}
		key := fmt.Sprintf("refusal-key-%04d", i)
		set(`DELETE FROM rate_counters`) // the hourly limit counts refusals too
		w := f.submit(f.me, key, c.body)
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.code) {
			t.Errorf("%s: %d %s, want %d %s", c.name, w.Code, w.Body, c.status, c.code)
		}
		if f.a.Count(`SELECT count(*) FROM orders`) != 0 || f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slot) != 0 ||
			f.a.Count(`SELECT count(*) FROM idempotency_keys`) != 0 || f.a.Count(`SELECT count(*) FROM outbox WHERE type LIKE 'order.%'`) != 0 {
			t.Fatalf("%s: a refused bill left something behind", c.name)
		}
		if c.undo != nil {
			c.undo()
		}
	}
}
