package order_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// BIZ-01: whatever the cart shows, the bill uses the price in our database.
func TestQuote_PricesComeFromTheDatabase(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 2, 1}) // the cart claims Rs 0.01
	before := f.a.Count(`SELECT booked FROM slots WHERE shop_id = $1`, f.shop)

	q := f.quote(f.body("", rice))
	if len(q.Lines) != 1 || q.Lines[0].UnitPriceCents != itemPrice || q.Lines[0].LineCents != 2*itemPrice || !q.Lines[0].PriceChanged {
		t.Fatalf("lines = %+v, want the database price and the changed flag", q.Lines)
	}
	if q.Totals.ItemsCents != 2*itemPrice || q.Totals.DeliveryFeeCents != deliveryFee || q.Totals.TotalCents != 2*itemPrice+deliveryFee {
		t.Errorf("totals = %+v", q.Totals)
	}
	if !q.Checks.ShopOpen || !q.Checks.InRange || !q.Checks.MinOrderMet || q.Checks.PhoneVerified {
		t.Errorf("checks = %+v", q.Checks)
	}
	if len(q.Slots) != 1 || q.Slots[0].Remaining != 10 || q.Delivery.FeeRule == "" {
		t.Errorf("slots = %+v, delivery = %+v", q.Slots, q.Delivery)
	}
	// A quote writes nothing.
	if f.a.Count(`SELECT booked FROM slots WHERE shop_id = $1`, f.shop) != before || f.a.Count(`SELECT count(*) FROM orders`) != 0 {
		t.Error("a quote changed the database")
	}
	// A price that moves in the database moves the next quote.
	f.a.Exec(`UPDATE shop_products SET online_price_cents = 110000 WHERE id = $1`, rice)
	if got := f.quote(f.body("", rice)).Lines[0].UnitPriceCents; got != 110000 {
		t.Errorf("price after the change = %d, want 110000", got)
	}
}

// BIZ-02: a quantity that is not 1 to the product's limit never reaches a bill.
func TestQuote_Quantities(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	for _, qty := range []int{-1, 0, 1_000_000, 21} { // 20 is the product's limit
		f.cart(f.tenant, cartLine{rice, qty, itemPrice})
		f.me.Expect(http.MethodPost, "/bills/quote", f.body("", rice), http.StatusConflict, "QTY_LIMIT")
	}
	f.cart(f.tenant, cartLine{rice, 20, itemPrice})
	if got := f.quote(f.body("", rice)).Lines[0].Qty; got != 20 {
		t.Errorf("qty = %d, want 20", got)
	}
	// The request has no quantity: sending one is refused, so it cannot be used to change the cart.
	bad := strings.Replace(f.body("", rice), `"subPref"`, `"qty":1,"subPref"`, 1)
	f.me.Expect(http.MethodPost, "/bills/quote", bad, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestQuote_LeavesOutWhatIsNotForSale(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	tea := f.product(f.shop, "Tea 400g", 50000, 51000, false)
	hidden := f.product(f.shop, "Hidden", 10000, 11000, true)
	f.a.Exec(`UPDATE products SET status = 'HIDDEN' WHERE name = 'Hidden'`)
	other, _ := f.readyShop("other")
	elsewhere := f.product(other, "Other shop item", 10000, 11000, true)

	f.cart(f.tenant, cartLine{rice, 1, itemPrice}, cartLine{tea, 1, 51000}, cartLine{hidden, 1, 11000}, cartLine{elsewhere, 1, 11000})
	q := f.quote(f.body("", rice))
	if len(q.Lines) != 1 || len(q.Excluded) != 3 || q.Totals.ItemsCents != itemPrice {
		t.Fatalf("lines %d, excluded %+v, items %d; want 1 line and 3 excluded", len(q.Lines), q.Excluded, q.Totals.ItemsCents)
	}
	for _, e := range q.Excluded {
		if e.Reason != "ITEM_UNAVAILABLE" {
			t.Errorf("excluded %+v", e)
		}
	}
	// Another shop's product never shows its name.
	for _, e := range q.Excluded {
		if e.ShopProductID == elsewhere && e.Name != "" {
			t.Errorf("another shop's product was named: %+v", e)
		}
	}
}

func TestQuote_ShopChecksShowInTheAnswer(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})

	f.a.Exec(`UPDATE shops SET accepting_orders = false WHERE id = $1`, f.shop)
	if q := f.quote(f.body("", rice)); q.Checks.ShopOpen {
		t.Error("a shop that stopped orders reads as open")
	}
	f.a.Exec(`UPDATE shops SET accepting_orders = true WHERE id = $1`, f.shop)
	f.a.Exec(`UPDATE shop_settings SET min_order_cents = 500000 WHERE shop_id = $1`, f.shop)
	if q := f.quote(f.body("", rice)); q.Checks.MinOrderMet {
		t.Error("the minimum order was not applied")
	}
	f.a.Exec(`UPDATE users SET phone = '+94771234567', phone_verified_at = now()`)
	if q := f.quote(f.body("", rice)); !q.Checks.PhoneVerified {
		t.Error("a verified phone reads as not verified")
	}
	f.a.Exec(`UPDATE delivery_fee_rules SET max_km = 0.5`)
	q := f.quote(f.body("", rice))
	if q.Checks.InRange || q.Totals.DeliveryFeeCents != 0 || q.Totals.ItemsCents != itemPrice {
		t.Errorf("out of range: checks %+v, totals %+v", q.Checks, q.Totals)
	}
}

func TestQuote_Slots(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	var open uuid.UUID
	if err := f.a.Env.Owner.QueryRow(t.Context(), `SELECT id FROM slots WHERE shop_id = $1`, f.shop).Scan(&open); err != nil {
		t.Fatal(err)
	}
	slotted := func(id uuid.UUID) string { return f.body(`"slotId":"`+id.String()+`"`, rice) }

	f.quote(slotted(open))
	full := f.slot(f.shop, 1)
	f.a.Exec(`UPDATE slots SET booked = 1 WHERE id = $1`, full)
	f.me.Expect(http.MethodPost, "/bills/quote", slotted(full), http.StatusConflict, "SLOT_FULL")
	closed := f.slot(f.shop, 5)
	f.a.Exec(`UPDATE slots SET status = 'CLOSED' WHERE id = $1`, closed)
	f.me.Expect(http.MethodPost, "/bills/quote", slotted(closed), http.StatusConflict, "SLOT_CLOSED")
	past := f.slot(f.shop, 5)
	f.a.Exec(`UPDATE slots SET cutoff_at = now() - interval '1 minute', starts_at = now() + interval '1 hour', ends_at = now() + interval '2 hours' WHERE id = $1`, past)
	f.me.Expect(http.MethodPost, "/bills/quote", slotted(past), http.StatusConflict, "SLOT_CLOSED")
	// A slot of another shop is not found, and a full slot is still listed with nothing remaining, a closed or late one is not offered.
	other, _ := f.readyShop("other")
	var theirs uuid.UUID
	if err := f.a.Env.Owner.QueryRow(t.Context(), `SELECT id FROM slots WHERE shop_id = $1`, other).Scan(&theirs); err != nil {
		t.Fatal(err)
	}
	f.me.Expect(http.MethodPost, "/bills/quote", slotted(theirs), http.StatusNotFound, "NOT_FOUND")
	remaining := map[uuid.UUID]int{}
	for _, s := range f.quote(f.body("", rice)).Slots {
		remaining[s.ID] = s.Remaining
	}
	if len(remaining) != 2 || remaining[full] != 0 || remaining[open] != 10 {
		t.Errorf("slots = %v, want the open slot and the full one; closed and past-cutoff slots are not offered", remaining)
	}
}
