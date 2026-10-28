package pricing_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"gp/internal/pricing"
	"gp/test/apptest"
)

type fixture struct {
	a     *apptest.App
	admin apptest.Client
	shop  uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	a := apptest.New(t)
	return fixture{a: a, admin: a.AsAdmin(), shop: a.SeedShop("alpha")}
}

func (f fixture) category(slug string) uuid.UUID {
	f.a.T.Helper()
	var id uuid.UUID
	if err := f.a.Env.Owner.QueryRow(f.a.T.Context(), `INSERT INTO categories (slug, name) VALUES ($1, $1) RETURNING id`, slug).Scan(&id); err != nil {
		f.a.T.Fatal(err)
	}
	return id
}

// sell puts a new product on sale in the shop through the API and returns the
// shop product id.
func (f fixture) sell(category *uuid.UUID, price int64) uuid.UUID {
	f.a.T.Helper()
	var product uuid.UUID
	if err := f.a.Env.Owner.QueryRow(f.a.T.Context(),
		`INSERT INTO products (name, category_id) VALUES ('Product ' || gen_random_uuid()::text, $1) RETURNING id`, category).Scan(&product); err != nil {
		f.a.T.Fatal(err)
	}
	w := f.admin.OK(http.MethodPost, "/admin/shops/"+f.shop.String()+"/products", fmt.Sprintf(`{"productId":"%s","shopPriceCents":%d}`, product, price))
	return apptest.Decode[struct{ ID uuid.UUID }](f.a.T, w).ID
}

func (f fixture) online(id uuid.UUID) int64 {
	f.a.T.Helper()
	var c int64
	if err := f.a.Env.Owner.QueryRow(f.a.T.Context(), `SELECT online_price_cents FROM shop_products WHERE id = $1`, id).Scan(&c); err != nil {
		f.a.T.Fatal(err)
	}
	return c
}

func (f fixture) setDefaultMarkup(bp int) {
	f.a.T.Helper()
	f.admin.OK(http.MethodPut, "/admin/shops/"+f.shop.String()+"/settings", fmt.Sprintf(
		`{"deliveryType":"SLOT","paymentProvider":"MANUAL","driverSource":"SHOP","notifyChannels":["SMS"],"markupBp":%d,`+
			`"maxLines":60,"reviewTimeoutMin":15,"revisionTimeoutMin":15,"paymentTimeoutMin":30}`, bp))
}

func (f fixture) setMarkups(body string) *httptest.ResponseRecorder {
	f.a.T.Helper()
	return f.admin.Call(http.MethodPut, "/admin/shops/"+f.shop.String()+"/category-markups", body)
}

func markupBody(pairs ...any) string {
	body := `{"markups":[`
	for i := 0; i < len(pairs); i += 2 {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"categoryId":"%v","markupBp":%v}`, pairs[i], pairs[i+1])
	}
	return body + "]}"
}

func TestReprice_ShopMarkupChangeUpdatesEveryPrice(t *testing.T) {
	f := newFixture(t)
	rice := f.category("rice")
	a, b := f.sell(&rice, 100000), f.sell(nil, 20000)
	if f.online(a) != 102500 || f.online(b) != 20500 {
		t.Fatalf("starting prices = %d, %d", f.online(a), f.online(b))
	}

	f.setDefaultMarkup(500)
	if f.online(a) != 105000 || f.online(b) != 21000 {
		t.Fatalf("after 5%% = %d, %d", f.online(a), f.online(b))
	}
	if n := f.a.Count(`SELECT count(*) FROM price_changes WHERE source = 'MARKUP' AND changed_by = $1 AND old_shop_price = new_shop_price`,
		f.admin.Session.Me.ID); n != 2 {
		t.Fatalf("MARKUP history rows = %d, want 2", n)
	}
	if n := f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'catalog.changed' AND payload->'shopProductIds' @> to_jsonb($1::text)`, a.String()); n != 2 { // create + reprice
		t.Fatalf("events naming the product = %d", n)
	}

	// Saving the same markup again changes nothing and says nothing.
	rows, events := f.a.Count(`SELECT count(*) FROM price_changes`), f.a.Count(`SELECT count(*) FROM outbox`)
	f.setDefaultMarkup(500)
	if f.a.Count(`SELECT count(*) FROM price_changes`) != rows || f.a.Count(`SELECT count(*) FROM outbox`) != events {
		t.Fatal("an unchanged markup wrote history or events")
	}
}

func TestReprice_CategoryMarkupWinsOverTheShopMarkup(t *testing.T) {
	f := newFixture(t)
	rice, tea := f.category("rice"), f.category("tea")
	inRice, inTea, none := f.sell(&rice, 100000), f.sell(&tea, 100000), f.sell(nil, 100000)

	w := f.setMarkups(markupBody(rice, 1000, tea, 0))
	if w.Code != http.StatusOK {
		t.Fatalf("set markups = %d %s", w.Code, w.Body)
	}
	if f.online(inRice) != 110000 || f.online(inTea) != 100000 || f.online(none) != 102500 {
		t.Fatalf("prices = %d, %d, %d", f.online(inRice), f.online(inTea), f.online(none))
	}

	// A product added later is priced with the same rule.
	if later := f.sell(&rice, 20000); f.online(later) != 22000 {
		t.Fatalf("new rice product = %d", f.online(later))
	}
	// The shop markup no longer touches a category that has its own.
	f.setDefaultMarkup(2000)
	if f.online(inRice) != 110000 || f.online(none) != 120000 {
		t.Fatalf("after shop markup 20%%: rice %d, none %d", f.online(inRice), f.online(none))
	}
	// Replacing the list with an empty one puts everything back on the shop markup.
	f.setMarkups(`{"markups":[]}`)
	if f.online(inRice) != 120000 || f.online(inTea) != 120000 {
		t.Fatalf("after clearing: %d, %d", f.online(inRice), f.online(inTea))
	}
	got := apptest.Decode[struct{ Data []pricing.Markup }](t, f.admin.OK(http.MethodGet, "/admin/shops/"+f.shop.String()+"/category-markups", ""))
	if len(got.Data) != 0 {
		t.Fatalf("markups left: %v", got.Data)
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'category_markups.set'`); n != 2 {
		t.Fatalf("audit rows = %d", n)
	}
}

func TestReprice_ProductWithoutAValidPriceKeepsTheOldOne(t *testing.T) {
	f := newFixture(t)
	good := f.sell(nil, 100000)
	// 100.40 at 0% rounds down under the shop price: no valid customer price.
	f.a.Exec(`WITH p AS (INSERT INTO products (name) VALUES ('Odd price') RETURNING id)
		INSERT INTO shop_products (shop_id, product_id, shop_price_cents, online_price_cents)
		SELECT $1, id, 10040, 10100 FROM p`, f.shop)

	f.setDefaultMarkup(0)
	if f.online(good) != 100000 {
		t.Fatalf("good product = %d", f.online(good))
	}
	if n := f.a.Count(`SELECT count(*) FROM shop_products WHERE shop_price_cents = 10040 AND online_price_cents = 10100`); n != 1 {
		t.Fatal("the product without a valid price was changed")
	}
}

func TestReprice_BigShopsSendSeveralEvents(t *testing.T) {
	f := newFixture(t)
	f.a.Exec(`WITH p AS (INSERT INTO products (name) SELECT 'Bulk ' || n FROM generate_series(1, 450) n RETURNING id)
		INSERT INTO shop_products (shop_id, product_id, shop_price_cents, online_price_cents) SELECT $1, id, 10000, 10250 FROM p`, f.shop)

	f.setDefaultMarkup(500)
	if n := f.a.Count(`SELECT count(*) FROM shop_products WHERE online_price_cents = 10500`); n != 450 {
		t.Fatalf("repriced = %d", n)
	}
	if n := f.a.Count(`SELECT count(*) FROM price_changes WHERE source = 'MARKUP'`); n != 450 {
		t.Fatalf("history rows = %d", n)
	}
	want := map[int]int{200: 2, 50: 1}
	got := map[int]int{}
	rows, err := f.a.Env.Owner.Query(f.a.T.Context(), `SELECT jsonb_array_length(payload->'shopProductIds') FROM outbox WHERE type = 'catalog.changed'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var n int
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		got[n]++
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events by size = %v, want %v", got, want)
	}
}

func TestSetMarkups_Rules(t *testing.T) {
	f := newFixture(t)
	rice, tea := f.category("rice"), f.category("tea")
	id := f.sell(&rice, 100000)
	f.setMarkups(markupBody(rice, 1000))

	tests := map[string]struct{ body, field, reason string }{
		"too high":         {markupBody(rice, 2001), "markups[0].markupBp", "out_of_range"},
		"negative":         {markupBody(rice, -1), "markups[0].markupBp", "out_of_range"},
		"twice":            {markupBody(rice, 100, rice, 200), "markups[1].categoryId", "invalid"},
		"unknown category": {markupBody(tea, 100, "00000000-0000-0000-0000-000000000001", 100), "markups", "unknown_category"},
		"no id":            {markupBody(uuid.Nil, 100), "markups[0].categoryId", "invalid"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			w := f.setMarkups(tc.body)
			e := apptest.ErrorOf(t, w)
			if w.Code != http.StatusBadRequest || e.Error.Fields[tc.field] != tc.reason {
				t.Fatalf("%d %s, want %s=%s", w.Code, w.Body, tc.field, tc.reason)
			}
		})
	}
	// A refused list changes nothing: the old markup still prices the product.
	if f.online(id) != 110000 || f.a.Count(`SELECT count(*) FROM category_markups`) != 1 {
		t.Fatalf("refused request changed data: online %d", f.online(id))
	}
	f.admin.Expect(http.MethodPut, "/admin/shops/00000000-0000-0000-0000-000000000001/category-markups", `{"markups":[]}`, http.StatusNotFound, "NOT_FOUND")
	f.admin.Expect(http.MethodGet, "/admin/shops/00000000-0000-0000-0000-000000000001/category-markups", "", http.StatusNotFound, "NOT_FOUND")
}
