package order_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

func (f fixture) list(c apptest.Client, shop uuid.UUID, query string) order.ConsolePage {
	f.a.T.Helper()
	return apptest.Decode[order.ConsolePage](f.a.T, c.OK(http.MethodGet, f.consolePath(shop, "")+query, ""))
}

func TestConsole_ListAndDetail(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	staff := f.staff("asha", "SHOP_STAFF")
	f.a.Exec(`UPDATE users SET name = 'Nimal Perera' WHERE email = 'nimal@example.com'`)

	page := f.list(staff, f.shop, "")
	if len(page.Data) != 1 || page.NextCursor != nil {
		t.Fatalf("list = %+v", page)
	}
	row := page.Data[0]
	if row.ID != x.o.ID || row.Status != enum.OrderStatusSubmitted || row.Version != 1 || row.ItemCount != 2 || row.Slot == nil ||
		row.TotalCents != x.o.Totals.TotalCents || row.Customer.Name != "Nimal Perera" || !strings.Contains(row.Customer.PhoneMasked, "*") {
		t.Errorf("row = %+v", row)
	}
	raw := staff.OK(http.MethodGet, f.consolePath(f.shop, ""), "").Body.String()
	if strings.Contains(raw, "+94771") {
		t.Error("the list shows a full phone number")
	}

	// Filters: a list of statuses, and a bad one is a bad request.
	if n := len(f.list(staff, f.shop, "?status=SUBMITTED,REVISED").Data); n != 1 {
		t.Errorf("status filter = %d rows, want 1", n)
	}
	if n := len(f.list(staff, f.shop, "?status=PAID").Data); n != 0 {
		t.Errorf("status PAID = %d rows, want 0", n)
	}
	for _, q := range []string{"?status=NOPE", "?status=SUBMITTED,,PAID", "?limit=x", "?cursor=junk"} {
		staff.Expect(http.MethodGet, f.consolePath(f.shop, "")+q, "", http.StatusBadRequest, "VALIDATION_ERROR")
	}

	d := apptest.Decode[order.Detail](t, staff.OK(http.MethodGet, f.consolePath(f.shop, "/"+x.o.ID.String()), ""))
	if d.ID != x.o.ID || d.Customer == nil || d.Customer.Phone != "" || d.Customer.PhoneMasked == "" || len(d.Actions) != 3 {
		t.Errorf("detail = %+v", d)
	}
	staff.Expect(http.MethodGet, f.consolePath(f.shop, "/"+uuid.NewString()), "", http.StatusNotFound, "NOT_FOUND")
	staff.Expect(http.MethodGet, f.consolePath(f.shop, "/not-an-id"), "", http.StatusNotFound, "NOT_FOUND")
}

func TestConsole_ListPages(t *testing.T) {
	f := newFixture(t)
	f.verify("nimal@example.com")
	rice := f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true)
	slot := f.slot(f.shop, 50)
	for i := 1; i <= 3; i++ {
		f.cart(f.tenant, cartLine{rice, i, itemPrice})
		f.placed(f.me, "console-key-"+strings.Repeat("a", i)+"0000000", f.body(`"slotId":"`+slot.String()+`"`, rice))
		f.a.Exec(`UPDATE orders SET status = 'DELIVERED'`)
	}
	staff := f.staff("asha", "SHOP_STAFF")
	first := f.list(staff, f.shop, "?limit=2")
	if len(first.Data) != 2 || first.NextCursor == nil {
		t.Fatalf("page 1 = %+v", first)
	}
	second := f.list(staff, f.shop, "?limit=2&cursor="+url.QueryEscape(*first.NextCursor))
	if len(second.Data) != 1 || second.NextCursor != nil || second.Data[0].ID == first.Data[0].ID || second.Data[0].ID == first.Data[1].ID {
		t.Errorf("page 2 = %+v", second)
	}
}

// The list carries an ETag; the same answer is 304, a changed one is a new tag.
func TestConsole_ListETag(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	staff := f.staff("asha", "SHOP_STAFF")
	path := f.consolePath(f.shop, "")

	first := staff.Call(http.MethodGet, path, "")
	tag := first.Header().Get("ETag")
	if first.Code != http.StatusOK || tag == "" {
		t.Fatalf("first = %d, ETag %q", first.Code, tag)
	}
	again := staff.CallWith(http.MethodGet, path, "", map[string]string{"If-None-Match": tag})
	if again.Code != http.StatusNotModified || again.Body.Len() != 0 || again.Header().Get("ETag") != tag {
		t.Errorf("same tag = %d, %d bytes, ETag %q; want 304, no body, same tag", again.Code, again.Body.Len(), again.Header().Get("ETag"))
	}
	if w := staff.CallWith(http.MethodGet, path, "", map[string]string{"If-None-Match": `W/` + tag + `, "other"`}); w.Code != http.StatusNotModified {
		t.Errorf("weak tag in a list = %d, want 304", w.Code)
	}
	if w := staff.CallWith(http.MethodGet, path, "", map[string]string{"If-None-Match": `"stale"`}); w.Code != http.StatusOK {
		t.Errorf("another tag = %d, want 200", w.Code)
	}

	f.review(staff, x.o.ID, version(1, `"action":"APPROVE"`))
	changed := staff.CallWith(http.MethodGet, path, "", map[string]string{"If-None-Match": tag})
	if changed.Code != http.StatusOK || changed.Header().Get("ETag") == tag {
		t.Errorf("after a change = %d, ETag %q; want 200 and a new tag", changed.Code, changed.Header().Get("ETag"))
	}
	// Errors never carry a tag, and a detail is not tagged.
	if tag := staff.Call(http.MethodGet, path+"?status=NOPE", "").Header().Get("ETag"); tag != "" {
		t.Errorf("an error carries ETag %q", tag)
	}
}

// AUTHZ-02: a member of shop A, an admin on the wrong path and a customer meet "not found" on every console order route.
func TestConsole_OtherShopIsNotFound(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	other, _ := f.readyShop("other")
	theirs := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "theirs", Email: "theirs@example.com", Name: "Theirs"}))
	f.a.Grant(theirs.Session.Me.ID, &other, "SHOP_OWNER")
	admin := f.a.AsAdmin()

	type call struct{ method, path, body string }
	routes := func(shop uuid.UUID) []call {
		id := x.o.ID.String()
		return []call{
			{http.MethodGet, f.consolePath(shop, ""), ""},
			{http.MethodGet, f.consolePath(shop, "/"+id), ""},
			{http.MethodPost, f.consolePath(shop, "/"+id+"/review"), version(1, `"action":"APPROVE"`)},
		}
	}
	// Their own shop's console, my order's id: the order is not theirs.
	for _, c := range routes(other)[1:] {
		theirs.Expect(c.method, c.path, c.body, http.StatusNotFound, "NOT_FOUND")
		admin.Expect(c.method, c.path, c.body, http.StatusNotFound, "NOT_FOUND")
	}
	// My shop's console: they are no member.
	for _, c := range routes(f.shop) {
		theirs.Expect(c.method, c.path, c.body, http.StatusNotFound, "NOT_FOUND")
		f.me.Expect(c.method, c.path, c.body, http.StatusNotFound, "NOT_FOUND")
	}
	if got := f.list(theirs, other, ""); len(got.Data) != 0 {
		t.Errorf("their list holds %d orders of another shop", len(got.Data))
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status = 'SUBMITTED' AND version = 1`, x.o.ID) != 1 {
		t.Fatal("a call from outside the shop changed the order")
	}
	// The admin may work any shop's console, on that shop's own path.
	if w := admin.Call(http.MethodPost, f.consolePath(f.shop, "/"+x.o.ID.String()+"/review"), version(1, `"action":"APPROVE"`)); w.Code != http.StatusOK {
		t.Errorf("admin on the right shop = %d %s", w.Code, w.Body)
	}
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'order.transition' AND entity_id = $1`, x.o.ID.String()) != 1 {
		t.Error("an admin's review is not audited")
	}
}
