package catalog_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/catalog"
	"gp/test/fakes"
)

func TestShopPage_ShowsTheShopAndCategoryCounts(t *testing.T) {
	f := newFixture(t)
	shop := f.activeShop("krishna-super", 6.9271, 79.8612)
	f.a.Exec(`UPDATE shop_settings SET min_order_cents = 100000, cart_tenant_id = 'ten_1', cart_public_key = 'pk_live_abc' WHERE shop_id = $1`, shop)
	f.a.Exec(`UPDATE shops SET accepting_orders = true, logo_url = 'https://img.example.test/SHOP_LOGO/a.png' WHERE id = $1`, shop)
	f.a.Exec(`INSERT INTO shop_hours (shop_id, weekday, opens, closes) SELECT $1, d, '08:00', '21:00' FROM generate_series(1, 7) d`, shop)
	rice, basmati, tea := f.category("rice"), f.category("basmati"), f.category("tea")
	f.a.Exec(`UPDATE categories SET parent_id = $1 WHERE id = $2`, rice.ID, basmati.ID)
	f.stock(shop, "Samba", 1000, nil, true, &rice.ID)
	f.stock(shop, "Basmati A", 1000, nil, true, &basmati.ID)
	f.stock(shop, "Basmati B", 1000, nil, false, &basmati.ID)
	f.stock(shop, "Tea", 1000, nil, true, &tea.ID)
	f.stock(shop, "Hidden", 1000, nil, true, &tea.ID)
	f.a.Exec(`UPDATE products SET status = 'HIDDEN' WHERE name = 'Hidden'`)
	other := f.activeShop("other", 6.9, 79.8)
	f.stock(other, "Elsewhere", 1000, nil, true, &tea.ID)

	w := f.publicGet("/shops/krishna-super")
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q", cc)
	}
	page := decode[catalog.ShopPage](f, w)
	if page.Slug != "krishna-super" || page.ID != shop || !page.AcceptingOrders || page.DeliveryType != "SLOT" || page.MinOrderCents != 100000 ||
		page.Cart == nil || page.Cart.TenantID != "ten_1" || page.Cart.PublicKey != "pk_live_abc" || page.LogoURL == nil {
		t.Fatalf("page = %+v", page)
	}
	if page.HoursToday == nil || page.HoursToday.Opens != "08:00" || page.HoursToday.Closes != "21:00" {
		t.Fatalf("hours today = %+v", page.HoursToday)
	}
	got := map[string]int{}
	for _, c := range page.Categories {
		got[c.Slug] = c.Count
	}
	if fmt.Sprint(got) != "map[basmati:2 rice:3 tea:1]" {
		t.Fatalf("category counts = %v", got)
	}
}

func TestShopPage_NothingPrivateAndNoCartYet(t *testing.T) {
	f := newFixture(t)
	f.activeShop("plain", 6.9271, 79.8612)
	w := f.publicGet("/shops/plain")
	page := decode[catalog.ShopPage](f, w)
	if page.Cart != nil || page.HoursToday != nil || page.OpenNow || len(page.Categories) != 0 || page.Categories == nil {
		t.Fatalf("page = %+v", page)
	}
	for _, banned := range []string{"lat", "lng", "phone", "payment", "markup", "secret", "status"} {
		if strings.Contains(w.Body.String(), `"`+banned) {
			t.Errorf("the page exposes %q: %s", banned, w.Body)
		}
	}
}

func TestShopPage_OnlyActiveShops(t *testing.T) {
	f := newFixture(t)
	f.a.SeedShop("draft")
	suspended := f.activeShop("suspended", 6.9, 79.8)
	f.a.Exec(`UPDATE shops SET status = 'SUSPENDED' WHERE id = $1`, suspended)
	for _, slug := range []string{"draft", "suspended", "nope"} {
		for _, path := range []string{"/shops/" + slug, "/shops/" + slug + "/products"} {
			w := f.publicGet(path)
			if w.Code != http.StatusNotFound || decodeErr(f, w).Error.Code != "NOT_FOUND" {
				t.Errorf("%s = %d %s", path, w.Code, w.Body)
			}
		}
	}
}

func productsWorld(t *testing.T) (fixture, uuid.UUID, uuid.UUID) {
	t.Helper()
	f := newFixture(t)
	shop := f.activeShop("krishna-super", 6.9271, 79.8612)
	rice, basmati := f.category("rice"), f.category("basmati")
	f.a.Exec(`UPDATE categories SET parent_id = $1 WHERE id = $2`, rice.ID, basmati.ID)
	f.stock(shop, "Samba Rice", 1000, ptr(int64(1200)), true, &rice.ID)
	f.stock(shop, "Basmati Rice", 2000, nil, true, &basmati.ID)
	f.stock(shop, "Sold out Rice", 3000, nil, false, &basmati.ID)
	f.stock(shop, "Tomato", 500, nil, true, nil)
	return f, shop, rice.ID
}

func listProducts(f fixture, query string) catalog.PublicPage[catalog.ProductCard] {
	f.a.T.Helper()
	return decode[catalog.PublicPage[catalog.ProductCard]](f, f.publicGet("/shops/krishna-super/products"+query))
}

func TestShopProducts_ByNameWithAvailabilityFlag(t *testing.T) {
	f, _, _ := productsWorld(t)
	w := f.publicGet("/shops/krishna-super/products")
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=60" {
		t.Errorf("Cache-Control = %q", cc)
	}
	page := decode[catalog.PublicPage[catalog.ProductCard]](f, w)
	var got []string
	for _, p := range page.Data {
		got = append(got, fmt.Sprintf("%s/%d/%v", p.Name, p.OnlinePriceCents, p.Available))
	}
	if fmt.Sprint(got) != "[Basmati Rice/2000/true Samba Rice/1000/true Sold out Rice/3000/false Tomato/500/true]" || page.NextCursor != nil {
		t.Fatalf("products = %v, cursor %v", got, page.NextCursor)
	}
	if p := page.Data[1]; p.MRPCents == nil || *p.MRPCents != 1200 || p.Brand == nil || p.MaxPerOrder != 20 || p.ShopSlug != "" {
		t.Fatalf("card = %+v", p)
	}
}

func TestShopProducts_CategoryIncludesChildren(t *testing.T) {
	f, _, _ := productsWorld(t)
	if n := len(listProducts(f, "?category=rice").Data); n != 3 {
		t.Errorf("rice (with its child) = %d, want 3", n)
	}
	if n := len(listProducts(f, "?category=basmati").Data); n != 2 {
		t.Errorf("basmati = %d, want 2", n)
	}
	if n := len(listProducts(f, "?category=unknown").Data); n != 0 {
		t.Errorf("unknown category = %d, want 0", n)
	}
	if got := listProducts(f, "?q=tomatto").Data; len(got) != 1 || got[0].Name != "Tomato" {
		t.Errorf("typo search in the shop = %+v", got)
	}
}

func TestShopProducts_PagesAndLimits(t *testing.T) {
	f, shop, _ := productsWorld(t)
	for i := 0; i < 30; i++ {
		f.stock(shop, fmt.Sprintf("Item %02d", i), 100, nil, true, nil)
	}
	first := listProducts(f, "?limit=10")
	if len(first.Data) != 10 || first.NextCursor == nil {
		t.Fatalf("first = %d rows, cursor %v", len(first.Data), first.NextCursor)
	}
	seen := map[uuid.UUID]bool{}
	cursor, pages := "", 0
	for {
		page := listProducts(f, "?limit=10&cursor="+cursor)
		for _, p := range page.Data {
			if seen[p.ShopProductID] {
				t.Fatalf("%s listed twice", p.Name)
			}
			seen[p.ShopProductID] = true
		}
		pages++
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if len(seen) != 34 || pages != 4 {
		t.Fatalf("saw %d products in %d pages", len(seen), pages)
	}
	for _, bad := range []string{"?limit=0", "?limit=51", "?limit=abc", "?cursor=junk", "?q=x"} {
		if w := f.publicGet("/shops/krishna-super/products" + bad); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d", bad, w.Code)
		}
	}
	if n := len(listProducts(f, "?limit=50").Data); n != 34 {
		t.Errorf("limit 50 = %d rows", n)
	}
}

func TestCategories_Tree(t *testing.T) {
	f := newFixture(t)
	rice, basmati, tea := f.category("rice"), f.category("basmati"), f.category("tea")
	f.a.Exec(`UPDATE categories SET parent_id = $1, sort = 1 WHERE id = $2`, rice.ID, basmati.ID)
	f.a.Exec(`UPDATE categories SET sort = 5 WHERE id = $1`, tea.ID)
	w := f.publicGet("/categories")
	if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=300" {
		t.Errorf("Cache-Control = %q", cc)
	}
	tree := decode[struct{ Data []catalog.CategoryNode }](f, w).Data
	if len(tree) != 2 || tree[0].Slug != "rice" || len(tree[0].Children) != 1 || tree[0].Children[0].Slug != "basmati" || tree[1].Children == nil {
		t.Fatalf("tree = %+v", tree)
	}
}

func TestBuyAgain_OnlyMyOwnPurchasesInThisShop(t *testing.T) {
	f := newFixture(t)
	shop := f.activeShop("krishna-super", 6.9271, 79.8612)
	other := f.activeShop("other", 6.9, 79.8)
	me := f.a.SignedIn(fakes.Claims{Sub: "me", Email: "me@example.test"})
	you := f.a.SignedIn(fakes.Claims{Sub: "you", Email: "you@example.test"})
	rice := f.stock(shop, "Rice", 1000, nil, true, nil)
	tea := f.stock(shop, "Tea", 2000, nil, false, nil)
	f.stock(shop, "Never bought", 3000, nil, true, nil)
	elsewhere := f.stock(other, "Elsewhere", 500, nil, true, nil)
	bought := func(user uuid.UUID, sp uuid.UUID, ago string) {
		f.a.Exec(`INSERT INTO customer_products (user_id, shop_id, product_id, last_bought_at)
			SELECT $1, shop_id, product_id, now() - $3::interval FROM shop_products WHERE id = $2`, user, sp, ago)
	}
	bought(me.Me.ID, rice, "3 days")
	bought(me.Me.ID, tea, "1 day")
	bought(me.Me.ID, elsewhere, "1 hour")
	bought(you.Me.ID, rice, "1 hour")
	f.a.Exec(`UPDATE shop_products SET online_price_cents = 1100 WHERE id = $1`, rice) // today's price, not the old one

	w := f.a.Call(http.MethodGet, "/v1/shops/krishna-super/buy-again", "", me.Cookies()...)
	got := decode[struct{ Data []catalog.ProductCard }](f, w).Data
	if len(got) != 2 || got[0].Name != "Tea" || got[0].Available || got[1].Name != "Rice" || got[1].OnlinePriceCents != 1100 {
		t.Fatalf("my list = %+v", got)
	}
	if cc := w.Header().Get("Cache-Control"); cc == "public, max-age=60" {
		t.Errorf("a private list is cacheable: %q", cc)
	}

	empty := decode[struct{ Data []catalog.ProductCard }](f, f.a.Call(http.MethodGet, "/v1/shops/other/buy-again", "", you.Cookies()...))
	if empty.Data == nil || len(empty.Data) != 0 {
		t.Fatalf("someone else's list = %+v", empty.Data)
	}
	if w := f.a.Call(http.MethodGet, "/v1/shops/nope/buy-again", "", me.Cookies()...); w.Code != http.StatusNotFound {
		t.Fatalf("unknown shop = %d", w.Code)
	}
	if w := f.publicGet("/shops/krishna-super/buy-again"); w.Code != http.StatusUnauthorized {
		t.Fatalf("signed out = %d", w.Code)
	}
}
