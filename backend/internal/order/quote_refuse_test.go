package order_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/test/apptest"
	"gp/test/fakes"
)

// Everything of another tenant, shop or customer is 404 or a plain refusal (CLAUDE.md rule 9).
func TestQuote_Isolation(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})

	// Another customer's address is not found.
	stranger := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "kamal", Email: "kamal@example.com", Name: "Kamal"}))
	theirs := f.address(stranger)
	f.me.Expect(http.MethodPost, "/bills/quote", strings.Replace(f.body("", rice), f.addr.String(), theirs.String(), 1), http.StatusNotFound, "NOT_FOUND")

	// A shop that does not exist, or is still a draft, is not found.
	f.me.Expect(http.MethodPost, "/bills/quote", strings.Replace(f.body("", rice), f.shop.String(), uuid.NewString(), 1), http.StatusNotFound, "NOT_FOUND")
	f.a.Exec(`UPDATE shops SET status = 'DRAFT' WHERE id = $1`, f.shop)
	f.me.Expect(http.MethodPost, "/bills/quote", f.body("", rice), http.StatusNotFound, "NOT_FOUND")
	f.a.Exec(`UPDATE shops SET status = 'ACTIVE' WHERE id = $1`, f.shop)

	// A cart that lives in another shop's tenant is the wrong shop's cart.
	other, otherTenant := f.readyShop("other")
	theirItem := f.product(other, "Theirs", 10000, 11000, true)
	f.a.Cart.PutCart(otherTenant, cartOf("crt_other", theirItem))
	f.me.Expect(http.MethodPost, "/bills/quote", strings.Replace(f.body("", rice), cartID, "crt_other", 1), http.StatusConflict, "CART_WRONG_SHOP")
	f.me.Expect(http.MethodPost, "/bills/quote", strings.Replace(f.body("", rice), cartID, "crt_missing", 1), http.StatusConflict, "CART_WRONG_SHOP")
}

func TestQuote_BadRequests(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})

	f.me.Expect(http.MethodPost, "/bills/quote", `{}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/bills/quote", strings.Replace(f.body("", rice), cartID, "../admin", 1), http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/bills/quote", f.body("", uuid.New()), http.StatusBadRequest, "VALIDATION_ERROR") // not in the cart
	f.me.Expect(http.MethodPost, "/bills/quote", strings.Replace(f.body("", rice), "SIMILAR", "ANYTHING", 1), http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/bills/quote", f.body("", rice, rice), http.StatusBadRequest, "VALIDATION_ERROR") // a line twice
	f.me.Expect(http.MethodPost, "/bills/quote", f.body(`"unitPrice":1`, rice), http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/bills/quote", f.body(`"tenantId":"x"`, rice), http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestQuote_CartProblems(t *testing.T) {
	f := newFixture(t)
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	f.cart(f.tenant) // an empty cart
	f.me.Expect(http.MethodPost, "/bills/quote", f.body(""), http.StatusConflict, "CART_EMPTY")

	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	f.a.Cart.FailNext(10, http.StatusBadGateway, "", "carts/")
	f.me.Expect(http.MethodPost, "/bills/quote", f.body("", rice), http.StatusServiceUnavailable, "CART_UNAVAILABLE")
	f.a.Cart.Heal()
	f.quote(f.body("", rice))

	// A shop whose cart is not switched on has no cart to read.
	f.a.Exec(`UPDATE shop_settings SET cart_tenant_id = NULL WHERE shop_id = $1`, f.shop)
	f.me.Expect(http.MethodPost, "/bills/quote", f.body("", rice), http.StatusServiceUnavailable, "CART_UNAVAILABLE")
}

func TestQuote_NeedsALogin(t *testing.T) {
	f := newFixture(t)
	w := f.a.Call(http.MethodPost, "/v1/bills/quote", f.body(""))
	if w.Code != http.StatusUnauthorized || apptest.ErrorOf(t, w).Error.Code != "UNAUTHENTICATED" {
		t.Errorf("no cookie = %d %s, want 401", w.Code, w.Body)
	}
}
