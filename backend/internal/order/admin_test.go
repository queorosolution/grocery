package order_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/test/apptest"
)

func (f fixture) adminList(query string) order.ConsolePage {
	f.a.T.Helper()
	return apptest.Decode[order.ConsolePage](f.a.T, f.admin().OK(http.MethodGet, "/admin/orders"+query, ""))
}

// FR-ADM-02: the admin finds orders of every shop by shop, status, code or the last four digits of the phone.
func TestAdminOrders_Search(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	f.secondOrderIn("other")

	if n := len(f.adminList("").Data); n != 2 {
		t.Fatalf("all orders = %d, want 2", n)
	}
	one := f.adminList("?shopId=" + f.shop.String())
	if len(one.Data) != 1 || one.Data[0].ID != x.o.ID || one.Data[0].ShopID != f.shop {
		t.Errorf("by shop = %+v", one.Data)
	}
	if n := len(f.adminList("?status=SUBMITTED").Data); n != 2 {
		t.Errorf("by status = %d", n)
	}
	if n := len(f.adminList("?status=PAID").Data); n != 0 {
		t.Errorf("by status PAID = %d", n)
	}
	byCode := f.adminList("?q=" + strings.ToLower(x.o.Code))
	if len(byCode.Data) != 1 || byCode.Data[0].ID != x.o.ID {
		t.Errorf("by code = %+v", byCode.Data)
	}
	var last4 string
	if err := f.a.Env.Owner.QueryRow(t.Context(), `SELECT right(address->>'phone', 4) FROM orders WHERE id = $1`, x.o.ID).Scan(&last4); err != nil {
		t.Fatal(err)
	}
	if n := len(f.adminList("?q=" + last4).Data); n < 1 {
		t.Errorf("by phone %s = %d", last4, n)
	}
	if raw := f.admin().OK(http.MethodGet, "/admin/orders", "").Body.String(); strings.Contains(raw, "+94771") {
		t.Error("the list shows a full phone number")
	}
	for _, q := range []string{"?q=%27%20OR%201=1", "?q=ab", "?shopId=junk", "?status=NOPE", "?limit=x"} {
		f.admin().Expect(http.MethodGet, "/admin/orders"+q, "", http.StatusBadRequest, "VALIDATION_ERROR")
	}
	// Shop staff and customers use their own lists.
	f.staff("ravi", "SHOP_STAFF").Expect(http.MethodGet, "/admin/orders", "", http.StatusForbidden, "FORBIDDEN")
	f.me.Expect(http.MethodGet, "/admin/orders", "", http.StatusForbidden, "FORBIDDEN")
}

// secondOrderIn makes a SUBMITTED order of another customer in a second shop.
func (f fixture) secondOrderIn(slug string) {
	f.a.T.Helper()
	shop, tenant := f.readyShop(slug)
	g := f
	g.shop, g.tenant = shop, tenant
	f = g
	c := f.a.As(f.a.SignedIn(claims("second")))
	f.a.Exec(`UPDATE users SET phone = '+94771234567', phone_verified_at = now() WHERE id = $1`, c.Session.Me.ID)
	addr := f.address(c)
	p := f.product(shop, "Dhal 1kg", 40000, 41000, true)
	f.cart(tenant, cartLine{p, 1, 41000})
	slot := f.firstSlot(shop)
	if w := f.submit(c, "second-key-00001", f.bodyOf(cartID, addr, `"slotId":"`+slot.String()+`"`, p)); w.Code != http.StatusCreated {
		f.a.T.Fatalf("second order = %d %s", w.Code, w.Body)
	}
}

// FR-ADM-03: the admin's override moves an order with a reason, and the move is audited.
func TestAdminOrders_Transition(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	a := f.admin()
	path := "/admin/orders/" + x.o.ID.String() + "/transition"
	a.Expect(http.MethodPost, path, version(1, `"to":"CANCELLED"`), http.StatusBadRequest, "VALIDATION_ERROR") // reason is needed
	a.Expect(http.MethodPost, path, version(1, `"to":"NOPE","reason":"because"`), http.StatusBadRequest, "VALIDATION_ERROR")
	a.Expect(http.MethodPost, path, version(1, `"to":"PAID","reason":"because"`), http.StatusBadRequest, "VALIDATION_ERROR") // mark-paid only
	a.Expect(http.MethodPost, path, version(9, `"to":"CANCELLED","reason":"because"`), http.StatusConflict, "STALE_ORDER")
	a.Expect(http.MethodPost, path, version(1, `"to":"PACKED","reason":"because"`), http.StatusConflict, "INVALID_TRANSITION")
	a.Expect(http.MethodPost, "/admin/orders/"+uuid.NewString()+"/transition", version(1, `"to":"CANCELLED","reason":"because"`), http.StatusNotFound, "NOT_FOUND")
	got := apptest.Decode[order.Detail](t, a.OK(http.MethodPost, path, version(1, `"to":"CANCELLED","reason":"Customer phoned"`)))
	if got.Status != "CANCELLED" || f.booked(x.slot) != 0 {
		t.Errorf("status %s, booked %d", got.Status, f.booked(x.slot))
	}
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'order.transition' AND actor_role = 'SUPER_ADMIN'`) != 1 {
		t.Error("the override is not audited")
	}
	f.staff("ravi", "SHOP_STAFF").Expect(http.MethodPost, path, version(2, `"to":"CANCELLED","reason":"because"`), http.StatusForbidden, "FORBIDDEN")
}
