package catalog_test

import (
	"net/http"
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
