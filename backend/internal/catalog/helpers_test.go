package catalog_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"gp/internal/catalog"
	"gp/test/apptest"
	"gp/test/fakes"
)

type fixture struct {
	a     *apptest.App
	admin apptest.Client
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	a := apptest.New(t)
	return fixture{a: a, admin: a.AsAdmin()}
}

func (f fixture) category(slug string) catalog.Category {
	f.a.T.Helper()
	w := f.admin.OK(http.MethodPost, "/admin/categories", `{"slug":"`+slug+`","name":"`+slug+`"}`)
	return apptest.Decode[catalog.Category](f.a.T, w)
}

func (f fixture) product(name string, category *uuid.UUID) catalog.Product {
	f.a.T.Helper()
	body := `{"name":"` + name + `"`
	if category != nil {
		body += `,"categoryId":"` + category.String() + `"`
	}
	w := f.admin.OK(http.MethodPost, "/admin/products", body+`}`)
	return apptest.Decode[catalog.Product](f.a.T, w)
}

// link puts the product on sale in the shop and returns the shop product.
func (f fixture) link(shopID, productID uuid.UUID, price int64, mrp *int64) catalog.ShopProduct {
	f.a.T.Helper()
	body := `{"productId":"` + productID.String() + `","shopPriceCents":` + strconv.FormatInt(price, 10)
	if mrp != nil {
		body += `,"mrpCents":` + strconv.FormatInt(*mrp, 10)
	}
	w := f.admin.OK(http.MethodPost, "/admin/shops/"+shopID.String()+"/products", body+`}`)
	return apptest.Decode[catalog.ShopProduct](f.a.T, w)
}

// owner signs in a shop owner of the shop.
func (f fixture) owner(sub string, shopID uuid.UUID) apptest.Client {
	f.a.T.Helper()
	s := f.a.SignedIn(fakes.Claims{Sub: sub, Email: sub + "@example.test"})
	f.a.Grant(s.Me.ID, &shopID, "SHOP_OWNER")
	return f.a.As(s)
}

func ptr[T any](v T) *T { return &v }

func (f fixture) shopProduct(c apptest.Client, shopID, id uuid.UUID) catalog.ShopProduct {
	f.a.T.Helper()
	w := c.OK(http.MethodGet, "/console/shops/"+shopID.String()+"/products", "")
	for _, sp := range apptest.Decode[catalog.Page[catalog.ShopProduct]](f.a.T, w).Data {
		if sp.ID == id {
			return sp
		}
	}
	f.a.T.Fatalf("shop product %s not listed", id)
	return catalog.ShopProduct{}
}

// publicGet calls a public route without signing in.
func (f fixture) publicGet(path string) *httptest.ResponseRecorder {
	return f.a.Call(http.MethodGet, "/v1"+path, "")
}

// activeShop seeds an ACTIVE shop at the given place.
func (f fixture) activeShop(slug string, lat, lng float64) uuid.UUID {
	f.a.T.Helper()
	id := f.a.SeedShop(slug)
	f.a.Exec(`UPDATE shops SET status = 'ACTIVE', lat = $2, lng = $3 WHERE id = $1`, id, lat, lng)
	return id
}

// stock puts a new product on sale in the shop straight in the database, with
// the customer price as given (the pricing engine is tested elsewhere).
func (f fixture) stock(shop uuid.UUID, name string, price int64, mrp *int64, available bool, category *uuid.UUID) uuid.UUID {
	f.a.T.Helper()
	var id uuid.UUID
	err := f.a.Env.Owner.QueryRow(f.a.T.Context(), `
		WITH p AS (INSERT INTO products (name, category_id, brand) VALUES ($1, $5, 'Brand') RETURNING id)
		INSERT INTO shop_products (shop_id, product_id, shop_price_cents, online_price_cents, mrp_cents, is_available)
		SELECT $2, id, $3, $3, $4, $6 FROM p RETURNING id`, name, shop, price, mrp, category, available).Scan(&id)
	if err != nil {
		f.a.T.Fatal(err)
	}
	return id
}

func decode[T any](f fixture, w *httptest.ResponseRecorder) T {
	f.a.T.Helper()
	if w.Code != http.StatusOK {
		f.a.T.Fatalf("status %d: %s", w.Code, w.Body)
	}
	return apptest.Decode[T](f.a.T, w)
}

func decodeErr(f fixture, w *httptest.ResponseRecorder) apptest.APIError {
	f.a.T.Helper()
	return apptest.ErrorOf(f.a.T, w)
}
