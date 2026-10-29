package catalog_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/catalog"
	"gp/test/apptest"
	"gp/test/fakes"
)

func consolePath(shopID, id uuid.UUID) string {
	return "/console/shops/" + shopID.String() + "/products/" + id.String()
}

func TestShopProduct_AddWorksOutTheCustomerPrice(t *testing.T) {
	f := newFixture(t)
	shop := f.a.SeedShop("alpha")
	p := f.product("Nadu Rice", nil)

	sp := f.link(shop, p.ID, 140000, ptr(int64(150000)))
	if sp.OnlinePriceCents != 143500 || sp.ShopPriceCents != 140000 || *sp.MRPCents != 150000 || sp.ManagedBy != "MANUAL" ||
		!sp.IsAvailable || sp.MaxPerOrder != 20 || sp.StockQty != nil || sp.Name != "Nadu Rice" {
		t.Fatalf("shop product = %+v", sp)
	}
	if n := f.a.Count(`SELECT count(*) FROM price_changes WHERE shop_product_id = $1 AND old_shop_price IS NULL
		AND new_shop_price = 140000 AND new_online_price = 143500 AND source = 'MANUAL'`, sp.ID); n != 1 {
		t.Fatalf("first price history rows = %d", n)
	}
	if n := f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'catalog.changed' AND shop_id = $1`, shop); n != 1 {
		t.Fatalf("catalog.changed events = %d", n)
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE entity = 'shop_product'`); n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
}

func TestShopProduct_AddRules(t *testing.T) {
	f := newFixture(t)
	shop := f.a.SeedShop("alpha")
	p := f.product("Rice", nil)
	f.link(shop, p.ID, 10000, nil)
	body := func(id uuid.UUID, rest string) string {
		return fmt.Sprintf(`{"productId":"%s"%s}`, id, rest)
	}
	tests := map[string]struct {
		body          string
		status        int
		field, reason string
	}{
		"already linked":    {body(p.ID, `,"shopPriceCents":10000`), 400, "productId", "taken"},
		"unknown product":   {body(uuid.MustParse(missing), `,"shopPriceCents":10000`), 400, "productId", "unknown"},
		"no price":          {body(p.ID, ``), 400, "shopPriceCents", "out_of_range"},
		"negative price":    {body(p.ID, `,"shopPriceCents":-5`), 400, "shopPriceCents", "out_of_range"},
		"mrp below price":   {body(p.ID, `,"shopPriceCents":10000,"mrpCents":9000`), 400, "mrpCents", "invalid_price"},
		"stock negative":    {body(p.ID, `,"shopPriceCents":10000,"stockQty":-1`), 400, "stockQty", "out_of_range"},
		"max per order 0":   {body(p.ID, `,"shopPriceCents":10000,"maxPerOrder":0`), 400, "maxPerOrder", "out_of_range"},
		"price in decimals": {body(p.ID, `,"shopPriceCents":100.5`), 400, "", ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := f.admin.Expect(http.MethodPost, "/admin/shops/"+shop.String()+"/products", tc.body, tc.status, "VALIDATION_ERROR")
			if tc.field != "" && e.Error.Fields[tc.field] != tc.reason {
				t.Fatalf("fields = %v, want %s=%s", e.Error.Fields, tc.field, tc.reason)
			}
		})
	}
	f.admin.Expect(http.MethodPost, "/admin/shops/"+missing+"/products", body(p.ID, `,"shopPriceCents":10000`), http.StatusNotFound, "NOT_FOUND")
	if n := f.a.Count(`SELECT count(*) FROM shop_products`); n != 1 {
		t.Fatalf("rejected requests left %d rows", n-1)
	}
}

func TestShopProduct_ConsoleEditChangesPriceAndKeepsHistory(t *testing.T) {
	f := newFixture(t)
	shop := f.a.SeedShop("alpha")
	owner := f.owner("owner", shop)
	sp := f.link(shop, f.product("Rice", nil).ID, 100000, ptr(int64(110000)))

	got := apptest.Decode[catalog.ShopProduct](t, owner.OK(http.MethodPatch, consolePath(shop, sp.ID), `{"shopPriceCents":104000}`))
	if got.ShopPriceCents != 104000 || got.OnlinePriceCents != 106600 {
		t.Fatalf("prices = %d / %d", got.ShopPriceCents, got.OnlinePriceCents)
	}
	if n := f.a.Count(`SELECT count(*) FROM price_changes WHERE shop_product_id = $1 AND old_shop_price = 100000
		AND new_shop_price = 104000 AND old_online_price = 102500 AND new_online_price = 106600 AND changed_by = $2`,
		sp.ID, owner.Session.Me.ID); n != 1 {
		t.Fatalf("price history rows = %d", n)
	}

	// The MRP caps the customer price; null removes the cap.
	got = apptest.Decode[catalog.ShopProduct](t, owner.OK(http.MethodPatch, consolePath(shop, sp.ID), `{"mrpCents":105000}`))
	if got.OnlinePriceCents != 105000 {
		t.Fatalf("capped price = %d", got.OnlinePriceCents)
	}
	got = apptest.Decode[catalog.ShopProduct](t, owner.OK(http.MethodPatch, consolePath(shop, sp.ID), `{"mrpCents":null}`))
	if got.MRPCents != nil || got.OnlinePriceCents != 106600 {
		t.Fatalf("uncapped = %+v", got)
	}

	before := f.a.Count(`SELECT count(*) FROM price_changes`)
	owner.OK(http.MethodPatch, consolePath(shop, sp.ID), `{"shopPriceCents":104000}`)
	if f.a.Count(`SELECT count(*) FROM price_changes`) != before {
		t.Fatal("a patch that changes nothing wrote price history")
	}
}

func TestShopProduct_ConsoleEditRules(t *testing.T) {
	f := newFixture(t)
	shop := f.a.SeedShop("alpha")
	owner := f.owner("owner", shop)
	sp := f.link(shop, f.product("Rice", nil).ID, 100000, ptr(int64(110000)))
	tests := map[string]struct{ body, field, reason string }{
		"mrp below shop price": {`{"mrpCents":90000}`, "mrpCents", "invalid_price"},
		"zero price":           {`{"shopPriceCents":0}`, "shopPriceCents", "out_of_range"},
		"stock too big":        {`{"stockQty":1000000001}`, "stockQty", "out_of_range"},
		"max per order 1000":   {`{"maxPerOrder":1000}`, "maxPerOrder", "out_of_range"},
		"unknown field":        {`{"onlinePriceCents":5}`, "", ""},
		"customer price text":  {`{"shopPriceCents":"12"}`, "", ""},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			e := owner.Expect(http.MethodPatch, consolePath(shop, sp.ID), tc.body, http.StatusBadRequest, "VALIDATION_ERROR")
			if tc.field != "" && e.Error.Fields[tc.field] != tc.reason {
				t.Fatalf("fields = %v, want %s=%s", e.Error.Fields, tc.field, tc.reason)
			}
		})
	}
	if got := f.shopProduct(owner, shop, sp.ID); got.OnlinePriceCents != sp.OnlinePriceCents {
		t.Fatalf("rejected edits changed the price to %d", got.OnlinePriceCents)
	}
}

func TestShopProduct_StockAvailabilityAndLimit(t *testing.T) {
	f := newFixture(t)
	shop := f.a.SeedShop("alpha")
	owner := f.owner("owner", shop)
	sp := f.link(shop, f.product("Rice", nil).ID, 100000, nil)
	patch := func(body string) catalog.ShopProduct {
		return apptest.Decode[catalog.ShopProduct](t, owner.OK(http.MethodPatch, consolePath(shop, sp.ID), body))
	}

	got := patch(`{"stockQty":12,"isAvailable":false,"maxPerOrder":5}`)
	if got.StockQty == nil || got.StockQty.String() != "12" || got.IsAvailable || got.MaxPerOrder != 5 {
		t.Fatalf("after edit = %+v", got)
	}
	if n := f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'catalog.changed'`); n != 2 { // create + this edit
		t.Fatalf("catalog.changed events = %d", n)
	}

	// A field that is left out keeps its value; null clears the stock.
	f.a.Exec(`UPDATE shop_products SET stock_qty = 2.5 WHERE id = $1`, sp.ID)
	if got = patch(`{"isAvailable":true}`); got.StockQty == nil || got.StockQty.String() != "2.5" {
		t.Fatalf("stock after unrelated edit = %v", got.StockQty)
	}
	if got = patch(`{"stockQty":null}`); got.StockQty != nil {
		t.Fatalf("stock after null = %v", got.StockQty)
	}
	if n := f.a.Count(`SELECT count(*) FROM price_changes`); n != 1 {
		t.Fatalf("non-price edits wrote price history: %d rows", n)
	}
}

// AUTHZ-08: what the shop database owns cannot be edited by hand.
func TestShopProduct_ConnectorFieldsAreRefused(t *testing.T) {
	f := newFixture(t)
	shop := f.a.SeedShop("alpha")
	owner := f.owner("owner", shop)
	sp := f.link(shop, f.product("Rice", nil).ID, 100000, nil)
	f.a.Exec(`UPDATE shop_products SET managed_by = 'CONNECTOR' WHERE id = $1`, sp.ID)

	for _, field := range []string{"shopPriceCents", "mrpCents", "stockQty", "isAvailable"} {
		value := map[string]string{"shopPriceCents": "5000", "mrpCents": "null", "stockQty": "3", "isAvailable": "false"}[field]
		e := owner.Expect(http.MethodPatch, consolePath(shop, sp.ID), `{"`+field+`":`+value+`}`, http.StatusBadRequest, "VALIDATION_ERROR")
		if e.Error.Fields[field] != "managed_by_connector" {
			t.Errorf("%s: fields = %v", field, e.Error.Fields)
		}
	}
	e := owner.Expect(http.MethodPatch, consolePath(shop, sp.ID), `{"shopPriceCents":5000,"maxPerOrder":3,"stockQty":1}`, http.StatusBadRequest, "VALIDATION_ERROR")
	if len(e.Error.Fields) != 2 {
		t.Errorf("every refused field should be named, got %v", e.Error.Fields)
	}
	got := apptest.Decode[catalog.ShopProduct](t, owner.OK(http.MethodPatch, consolePath(shop, sp.ID), `{"maxPerOrder":3}`))
	if got.MaxPerOrder != 3 || got.ShopPriceCents != 100000 || got.ManagedBy != "CONNECTOR" {
		t.Fatalf("max per order edit = %+v", got)
	}
}

func TestShopProduct_ShopsAreIsolated(t *testing.T) {
	f := newFixture(t)
	alpha, beta := f.a.SeedShop("alpha"), f.a.SeedShop("beta")
	ownerA, ownerB := f.owner("owner-a", alpha), f.owner("owner-b", beta)
	spA := f.link(alpha, f.product("Alpha Rice", nil).ID, 100000, nil)
	f.link(beta, f.product("Beta Tea", nil).ID, 50000, nil)

	list := apptest.Decode[catalog.Page[catalog.ShopProduct]](t, ownerA.OK(http.MethodGet, "/console/shops/"+alpha.String()+"/products", ""))
	if len(list.Data) != 1 || list.Data[0].Name != "Alpha Rice" {
		t.Fatalf("shop A sees %+v", list.Data)
	}
	// Beta's owner cannot reach alpha's shop at all, nor alpha's product through beta's route.
	ownerB.Expect(http.MethodGet, "/console/shops/"+alpha.String()+"/products", "", http.StatusNotFound, "NOT_FOUND")
	ownerB.Expect(http.MethodPatch, consolePath(alpha, spA.ID), `{"maxPerOrder":1}`, http.StatusNotFound, "NOT_FOUND")
	ownerB.Expect(http.MethodPatch, consolePath(beta, spA.ID), `{"shopPriceCents":1}`, http.StatusNotFound, "NOT_FOUND")
	if got := f.shopProduct(ownerA, alpha, spA.ID); got.ShopPriceCents != 100000 || got.MaxPerOrder != 20 {
		t.Fatalf("product changed: %+v", got)
	}
}

func TestShopProduct_StaffNeedTheEditRight(t *testing.T) {
	f := newFixture(t)
	shop := f.a.SeedShop("alpha")
	staff := f.a.SignedIn(fakes.Claims{Sub: "staff", Email: "staff@example.test"})
	f.a.Grant(staff.Me.ID, &shop, "SHOP_STAFF")
	c := f.a.As(staff)
	sp := f.link(shop, f.product("Rice", nil).ID, 100000, nil)

	c.OK(http.MethodGet, "/console/shops/"+shop.String()+"/products", "")
	c.Expect(http.MethodPatch, consolePath(shop, sp.ID), `{"maxPerOrder":3}`, http.StatusForbidden, "FORBIDDEN")
	f.a.Exec(`UPDATE memberships SET can_edit_products = true WHERE user_id = $1`, staff.Me.ID)
	c.OK(http.MethodPatch, consolePath(shop, sp.ID), `{"maxPerOrder":3}`)
}

func TestShopProduct_ListFiltersAndPages(t *testing.T) {
	f := newFixture(t)
	shop := f.a.SeedShop("alpha")
	owner := f.owner("owner", shop)
	for i := 0; i < 52; i++ {
		f.a.Exec(`WITH p AS (INSERT INTO products (name) VALUES ($1) RETURNING id)
			INSERT INTO shop_products (shop_id, product_id, shop_price_cents, online_price_cents, is_available)
			SELECT $2, id, 1000, 1100, $3 FROM p`, fmt.Sprintf("Item %02d", i), shop, i%4 != 0)
	}
	path := "/console/shops/" + shop.String() + "/products"
	page := func(q string) catalog.Page[catalog.ShopProduct] {
		return apptest.Decode[catalog.Page[catalog.ShopProduct]](t, owner.OK(http.MethodGet, path+q, ""))
	}
	first := page("")
	second := page("?cursor=" + first.NextCursor)
	if len(first.Data) != 50 || len(second.Data) != 2 || second.NextCursor != "" || second.Data[0].Name != "Item 50" {
		t.Fatalf("pages = %d + %d rows, last cursor %q", len(first.Data), len(second.Data), second.NextCursor)
	}
	if n := len(page("?available=false").Data); n != 13 {
		t.Errorf("unavailable = %d, want 13", n)
	}
	if n := len(page("?q=item+07").Data); n != 1 {
		t.Errorf("search = %d, want 1", n)
	}
	owner.Expect(http.MethodGet, path+"?available=maybe", "", http.StatusBadRequest, "VALIDATION_ERROR")
}
