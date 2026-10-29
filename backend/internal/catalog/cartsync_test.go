package catalog_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shop"
	"gp/test/apptest"
)

// cartShop is a shop with n products on sale and an admin client.
type cartShop struct {
	fixture
	id uuid.UUID
}

func newCartShop(t *testing.T, n int) cartShop {
	t.Helper()
	f := newFixture(t)
	id := f.a.SeedShop("krishna")
	if n > 0 {
		f.a.Exec(`INSERT INTO products (name) SELECT 'Item ' || g FROM generate_series(1, $1) g`, n)
		f.a.Exec(`
			INSERT INTO shop_products (shop_id, product_id, shop_price_cents, online_price_cents)
			SELECT $1, id, 100000, 100000 FROM products`, id)
	}
	return cartShop{fixture: f, id: id}
}

func (c cartShop) path(rest string) string { return "/admin/shops/" + c.id.String() + rest }

func (c cartShop) enable() shop.Detail {
	c.a.T.Helper()
	return apptest.Decode[shop.Detail](c.a.T, c.admin.OK(http.MethodPost, c.path("/cart-tenant"), ""))
}

func (c cartShop) drain() {
	c.a.T.Helper()
	if err := c.a.DrainEvents(); err != nil {
		c.a.T.Fatal(err)
	}
}

// changed writes a catalog.changed event the way the pricing module does.
func (c cartShop) changed(ids ...uuid.UUID) {
	c.a.T.Helper()
	err := c.a.Env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		return events.AddCatalogChanged(ctx, tx, c.id, ids)
	})
	if err != nil {
		c.a.T.Fatal(err)
	}
}

func (c cartShop) tenant() string {
	c.a.T.Helper()
	id, ok := c.a.Cart.TenantFor(c.id.String())
	if !ok {
		c.a.T.Fatal("no cart tenant was created for the shop")
	}
	return id
}

func TestEnableCart_CreatesTenantAndUploadsThePriceList(t *testing.T) {
	c := newCartShop(t, 3)
	d := c.enable()

	tenant := c.tenant()
	if d.Settings.CartTenantID == nil || *d.Settings.CartTenantID != tenant {
		t.Errorf("cartTenantId = %v, want %s", d.Settings.CartTenantID, tenant)
	}
	if d.Settings.CartPublicKey == nil || !strings.HasPrefix(*d.Settings.CartPublicKey, "pk_") {
		t.Errorf("cartPublicKey = %v, want a pk_ key", d.Settings.CartPublicKey)
	}
	if n := c.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'shop.cart_tenant.create' AND shop_id = $1`, c.id); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
	// Only the public key is kept anywhere: a secret key never reaches our database.
	if n := c.a.Count(`SELECT count(*) FROM shop_settings WHERE cart_public_key LIKE 'sk\_%'`); n != 0 {
		t.Errorf("a secret key was stored")
	}
	if n := c.a.Count(`SELECT count(*) FROM audit_log WHERE after::text LIKE '%\_key%' OR after::text LIKE '%pk\_%' OR after::text LIKE '%sk\_%'`); n != 0 {
		t.Errorf("a key was written to the audit log")
	}

	if got := len(c.a.Cart.Items(tenant)); got != 0 {
		t.Fatalf("items before the worker ran = %d, want 0 (the upload is a queued event)", got)
	}
	c.drain()
	items := c.a.Cart.Items(tenant)
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	for id, it := range items {
		if it.UnitPrice != 100000 || !it.Active || it.MaxQuantity != 20 || !strings.HasPrefix(it.Name, "Item ") {
			t.Errorf("item %s = %+v", id, it)
		}
	}
}

func TestEnableCart_IsIdempotent(t *testing.T) {
	c := newCartShop(t, 2)
	first := c.enable()
	events := c.a.Count(`SELECT count(*) FROM outbox`)

	second := c.enable()
	if *second.Settings.CartTenantID != *first.Settings.CartTenantID || *second.Settings.CartPublicKey != *first.Settings.CartPublicKey {
		t.Errorf("the second call changed the tenant: %+v vs %+v", first.Settings, second.Settings)
	}
	if got := len(c.a.Cart.TenantIDs()); got != 1 {
		t.Errorf("tenants = %d, want 1", got)
	}
	if n := c.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'shop.cart_tenant.create'`); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
	if n := c.a.Count(`SELECT count(*) FROM outbox`); n != events {
		t.Errorf("a repeat queued %d more events", n-events)
	}
}

func TestEnableCart_ReusesATenantFromAnEarlierFailedAttempt(t *testing.T) {
	c := newCartShop(t, 1)
	// The tenant was created but the first call died before we saved it.
	tenant, pk := c.a.Cart.AddTenant(c.id.String())

	d := c.enable()
	if *d.Settings.CartTenantID != tenant || *d.Settings.CartPublicKey != pk {
		t.Errorf("settings = %+v, want the existing tenant %s", d.Settings, tenant)
	}
	if got := len(c.a.Cart.TenantIDs()); got != 1 {
		t.Errorf("tenants = %d, want 1 (no duplicate)", got)
	}
}

func TestEnableCart_CartServiceDown(t *testing.T) {
	c := newCartShop(t, 1)
	c.a.Cart.FailNext(20, http.StatusServiceUnavailable, "", "/admin/")

	c.admin.Expect(http.MethodPost, c.path("/cart-tenant"), "", http.StatusServiceUnavailable, "CART_UNAVAILABLE")

	if n := c.a.Count(`SELECT count(*) FROM shop_settings WHERE shop_id = $1 AND cart_tenant_id IS NOT NULL`, c.id); n != 0 {
		t.Error("a tenant was saved although the cart service failed")
	}
	if n := c.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'shop.cart_tenant.create'`); n != 0 {
		t.Error("an audit row was written for a failed call")
	}
	if n := c.a.Count(`SELECT count(*) FROM outbox WHERE type = 'catalog.changed'`); n != 0 {
		t.Error("a price upload was queued for a failed call")
	}
}

func TestEnableCart_UnknownShop(t *testing.T) {
	f := newFixture(t)
	f.admin.Expect(http.MethodPost, "/admin/shops/"+uuid.NewString()+"/cart-tenant", "", http.StatusNotFound, "NOT_FOUND")
	if got := len(f.a.Cart.TenantIDs()); got != 0 {
		t.Errorf("a tenant was created for an unknown shop")
	}
}

func TestCartSync_UploadsInChunksOf25(t *testing.T) {
	c := newCartShop(t, 60)
	c.enable()
	c.drain()

	var sizes []int
	for _, b := range c.a.Cart.Batches() {
		sizes = append(sizes, len(b.Items))
	}
	want := []int{25, 25, 10}
	// 60 items arrive in one 200-id event, then are split into cart batches.
	if len(sizes) != len(want) || sizes[0] != 25 || sizes[1] != 25 || sizes[2] != 10 {
		t.Errorf("batch sizes = %v, want %v", sizes, want)
	}
	if got := len(c.a.Cart.Items(c.tenant())); got != 60 {
		t.Errorf("items = %d, want 60", got)
	}
}

func TestCartSync_FollowsPriceAndAvailability(t *testing.T) {
	c := newCartShop(t, 0)
	cat := c.category("rice")
	p := c.product("Samba Rice 5kg", &cat.ID)
	sp := c.link(c.id, p.ID, 100000, nil)
	c.enable()
	c.drain()
	tenant := c.tenant()
	if got := c.a.Cart.Items(tenant)[sp.ID.String()]; got.UnitPrice != sp.OnlinePriceCents || got.Name != "Samba Rice 5kg" || !got.Active {
		t.Fatalf("first upload = %+v", got)
	}

	owner := c.owner("owner-1", c.id)
	owner.OK(http.MethodPatch, "/console/shops/"+c.id.String()+"/products/"+sp.ID.String(), `{"shopPriceCents":120000,"isAvailable":false}`)
	c.drain()

	got := c.a.Cart.Items(tenant)[sp.ID.String()]
	if got.UnitPrice <= sp.OnlinePriceCents || got.Active {
		t.Errorf("after the change = %+v, want a higher price and inactive", got)
	}

	// A hidden master product is not for sale either.
	owner.OK(http.MethodPatch, "/console/shops/"+c.id.String()+"/products/"+sp.ID.String(), `{"isAvailable":true}`)
	c.admin.OK(http.MethodPatch, "/admin/products/"+p.ID.String(), `{"status":"HIDDEN"}`)
	c.changed(sp.ID)
	c.drain()
	if got := c.a.Cart.Items(tenant)[sp.ID.String()]; got.Active {
		t.Errorf("a hidden product is still active: %+v", got)
	}
}

func TestCartSync_NothingHappensWithoutATenant(t *testing.T) {
	c := newCartShop(t, 2)
	c.changed(uuid.New())
	c.drain()
	if calls := c.a.Cart.Calls(); len(calls) != 0 {
		t.Errorf("the cart service was called %d times for a shop without a cart", len(calls))
	}
}

func TestCartSync_RetriesAFiveHundred(t *testing.T) {
	c := newCartShop(t, 3)
	c.enable()
	c.a.Cart.FailNext(1, http.StatusServiceUnavailable, "", "/items/batch")
	c.drain()
	if got := len(c.a.Cart.Items(c.tenant())); got != 3 {
		t.Errorf("items after a retried 503 = %d, want 3", got)
	}
}

func TestCartSync_ALongOutageFailsTheEventAndTheNextDeliveryRecovers(t *testing.T) {
	c := newCartShop(t, 3)
	c.enable()
	c.a.Cart.FailNext(100, http.StatusServiceUnavailable, "", "/items/batch")
	if err := c.a.DrainEvents(); err == nil {
		t.Fatal("the consumer reported success during an outage")
	}
	if got := len(c.a.Cart.Items(c.tenant())); got != 0 {
		t.Fatalf("items during the outage = %d, want 0", got)
	}

	c.a.Cart.Heal()
	c.drain() // the queue delivers the same event again
	if got := len(c.a.Cart.Items(c.tenant())); got != 3 {
		t.Errorf("items after recovery = %d, want 3", got)
	}
}

func TestCartSync_SkipsItemsTheCartServiceWouldRefuseAndSendsTheRest(t *testing.T) {
	c := newCartShop(t, 3)
	// Over the cart service's price limit; the catalog API cannot create this, a bad import could.
	c.a.Exec(`UPDATE shop_products SET shop_price_cents = 2000000000, online_price_cents = 2000000000
	          WHERE id = (SELECT id FROM shop_products WHERE shop_id = $1 ORDER BY id LIMIT 1)`, c.id)
	c.enable()
	c.drain()
	if got := len(c.a.Cart.Items(c.tenant())); got != 2 {
		t.Errorf("items = %d, want the 2 valid ones", got)
	}
}

func TestCartSync_IgnoresProductsOfAnotherShop(t *testing.T) {
	c := newCartShop(t, 1)
	other := c.a.SeedShop("other")
	c.a.Exec(`INSERT INTO shop_products (shop_id, product_id, shop_price_cents, online_price_cents)
	          SELECT $1, id, 5000, 5000 FROM products`, other)
	var foreign uuid.UUID
	if err := c.a.Env.Owner.QueryRow(context.Background(), `SELECT id FROM shop_products WHERE shop_id = $1`, other).Scan(&foreign); err != nil {
		t.Fatal(err)
	}
	c.enable()
	c.drain()

	c.changed(foreign) // an event of this shop naming a product of another shop
	c.drain()
	if _, ok := c.a.Cart.Items(c.tenant())[foreign.String()]; ok {
		t.Error("a product of another shop was uploaded to this shop's tenant")
	}
}

func TestResyncCart(t *testing.T) {
	c := newCartShop(t, 4)

	c.admin.Expect(http.MethodPost, c.path("/cart-sync"), "", http.StatusConflict, "CONFLICT") // not enabled yet
	c.admin.Expect(http.MethodPost, "/admin/shops/"+uuid.NewString()+"/cart-sync", "", http.StatusNotFound, "NOT_FOUND")

	c.enable()
	c.drain()
	tenant := c.tenant()
	c.a.Exec(`UPDATE shop_products SET online_price_cents = 150000, shop_price_cents = 150000 WHERE shop_id = $1`, c.id) // drifted, no event
	w := c.admin.Call(http.MethodPost, c.path("/cart-sync"), "")
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), `"queuedItems":4`) {
		t.Fatalf("resync = %d %s", w.Code, w.Body)
	}
	c.drain()
	for id, it := range c.a.Cart.Items(tenant) {
		if it.UnitPrice != 150000 {
			t.Errorf("item %s price = %d after resync, want 150000", id, it.UnitPrice)
		}
	}
}
