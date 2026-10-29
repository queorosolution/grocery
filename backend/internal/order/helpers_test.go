package order_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/cartclient"
	"gp/internal/order"
	"gp/internal/user"
	"gp/test/apptest"
	"gp/test/fakes"
)

const (
	itemPrice    = 102500 // online price of the first product, in cents
	itemShop     = 100000
	deliveryFee  = 15000
	cartID       = "crt_test1"
	nearbyPinSQL = `"lat":6.93,"lng":79.87` // about 1.2 km from the seeded shop
)

type fixture struct {
	a      *apptest.App
	me     apptest.Client
	shop   uuid.UUID
	tenant string
	addr   uuid.UUID
}

// newFixture makes a shop that can take orders now (open all week, a payment
// account, a slot tomorrow, a fee rule, a cart tenant) and a customer with one address.
func newFixture(t *testing.T) fixture {
	t.Helper()
	a := apptest.New(t)
	f := fixture{a: a}
	f.shop, f.tenant = f.readyShop("krishna")
	f.me = a.As(a.SignedIn(fakes.Claims{Sub: "nimal", Email: "nimal@example.com", Name: "Nimal"}))
	f.addr = f.address(f.me)
	a.Exec(`INSERT INTO delivery_fee_rules (shop_id, base_cents) VALUES (NULL, $1)`, deliveryFee)
	return f
}

func (f fixture) readyShop(slug string) (uuid.UUID, string) {
	f.a.T.Helper()
	id := f.a.SeedShop(slug)
	tenant, _ := f.a.Cart.AddTenant(slug)
	f.a.Exec(`UPDATE shops SET status = 'ACTIVE', accepting_orders = true WHERE id = $1`, id)
	for day := 1; day <= 7; day++ {
		f.a.Exec(`INSERT INTO shop_hours (shop_id, weekday, opens, closes) VALUES ($1, $2, '00:00', '24:00')`, id, day)
	}
	f.a.Exec(`INSERT INTO payment_accounts (owner_type, provider, status) SELECT 'PLATFORM', 'MANUAL', 'ACTIVE'
		WHERE NOT EXISTS (SELECT 1 FROM payment_accounts WHERE owner_type = 'PLATFORM' AND provider = 'MANUAL')`)
	f.a.Exec(`UPDATE shop_settings SET payment_provider = 'MANUAL', cart_tenant_id = $2,
		payment_account_id = (SELECT id FROM payment_accounts WHERE owner_type = 'PLATFORM' AND provider = 'MANUAL')
		WHERE shop_id = $1`, id, tenant)
	f.slot(id, 10)
	return id, tenant
}

// slot adds a window tomorrow, open, with the given capacity.
func (f fixture) slot(shopID uuid.UUID, capacity int) uuid.UUID {
	f.a.T.Helper()
	var id uuid.UUID
	err := f.a.Env.Owner.QueryRow(context.Background(), `INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity)
		VALUES ($1, now() + interval '1 day' + make_interval(hours => $3::int), now() + interval '1 day 2 hours' + make_interval(hours => $3::int),
		        now() + interval '20 hours', $2) RETURNING id`, shopID, capacity, f.a.Count(`SELECT count(*) FROM slots`)).Scan(&id)
	if err != nil {
		f.a.T.Fatal(err)
	}
	return id
}

func (f fixture) address(c apptest.Client) uuid.UUID {
	f.a.T.Helper()
	w := c.OK(http.MethodPost, "/me/addresses", `{"label":"Home","line1":"12 Galle Road, Colombo 03",`+nearbyPinSQL+`}`)
	return apptest.Decode[user.AddressResult](f.a.T, w).Address.ID
}

// product puts a new product on sale in the shop and returns the shop product id.
func (f fixture) product(shopID uuid.UUID, name string, shopPrice, online int64, available bool) uuid.UUID {
	f.a.T.Helper()
	var id uuid.UUID
	err := f.a.Env.Owner.QueryRow(context.Background(), `
		WITH p AS (INSERT INTO products (name) VALUES ($2) RETURNING id)
		INSERT INTO shop_products (shop_id, product_id, shop_price_cents, online_price_cents, is_available)
		SELECT $1, p.id, $3, $4, $5 FROM p RETURNING id`, shopID, name, shopPrice, online, available).Scan(&id)
	if err != nil {
		f.a.T.Fatal(err)
	}
	return id
}

// cart puts a cart in the shop's tenant. Each line is {shop product id, quantity, the price the cart shows}.
func (f fixture) cart(tenant string, lines ...cartLine) {
	f.a.T.Helper()
	c := cartclient.Cart{ID: cartID, Status: "open", Currency: "LKR"}
	for i, l := range lines {
		id := l.id.String()
		c.Lines = append(c.Lines, cartclient.Line{ID: fmt.Sprintf("ln_%d", i), ItemID: &id, Name: "x", UnitPrice: l.shown, Quantity: l.qty})
	}
	f.a.Cart.PutCart(tenant, c)
}

type cartLine struct {
	id    uuid.UUID
	qty   int
	shown int64
}

// body is a quote request for the fixture's shop; extra goes before the closing brace.
func (f fixture) body(extra string, items ...uuid.UUID) string {
	refs := ""
	for i, id := range items {
		if i > 0 {
			refs += ","
		}
		refs += `{"itemId":"` + id.String() + `","subPref":"SIMILAR"}`
	}
	b := `{"shopId":"` + f.shop.String() + `","cartId":"` + cartID + `","addressId":"` + f.addr.String() + `","lines":[` + refs + `]`
	if extra != "" {
		b += "," + extra
	}
	return b + "}"
}

func (f fixture) quote(body string) order.Quote {
	f.a.T.Helper()
	return apptest.Decode[order.Quote](f.a.T, f.me.OK(http.MethodPost, "/bills/quote", body))
}

// cartOf is an open cart with one of each product, for the carts other tests fill by hand.
func cartOf(id string, items ...uuid.UUID) cartclient.Cart {
	c := cartclient.Cart{ID: id, Status: "open", Currency: "LKR"}
	for i, item := range items {
		s := item.String()
		c.Lines = append(c.Lines, cartclient.Line{ID: fmt.Sprintf("ln_%d", i), ItemID: &s, Name: "x", UnitPrice: 11000, Quantity: 1})
	}
	return c
}
