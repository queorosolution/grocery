package order_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/enum"
	"gp/test/apptest"
	"gp/test/fakes"
)

const sugarShop, sugarPrice = 20000, 20500

// trio is a submitted order with two lines (1 rice, 2 sugar), the products and the slot it booked.
type trio struct {
	o           order.Detail
	rice, sugar uuid.UUID
	slot        uuid.UUID
	rice0, sug0 order.LineView
}

func (f fixture) trio() trio {
	f.a.T.Helper()
	f.verify("nimal@example.com")
	t := trio{
		rice:  f.product(f.shop, "Samba Rice 5kg", itemShop, itemPrice, true),
		sugar: f.product(f.shop, "White Sugar 1kg", sugarShop, sugarPrice, true),
		slot:  f.slot(f.shop, 10),
	}
	f.cart(f.tenant, cartLine{t.rice, 1, itemPrice}, cartLine{t.sugar, 2, sugarPrice})
	t.o = f.placed(f.me, key1, f.body(`"slotId":"`+t.slot.String()+`"`, t.rice, t.sugar))
	for _, l := range t.o.Lines {
		if l.Name == "Samba Rice 5kg" {
			t.rice0 = l
		} else {
			t.sug0 = l
		}
	}
	return t
}

// staff signs in a member of the fixture's shop.
func (f fixture) staff(sub, role string) apptest.Client {
	f.a.T.Helper()
	c := f.a.As(f.a.SignedIn(fakes.Claims{Sub: sub, Email: sub + "@example.com", Name: sub}))
	f.a.Grant(c.Session.Me.ID, &f.shop, role)
	return c
}

func (f fixture) consolePath(shop uuid.UUID, rest string) string {
	return "/console/shops/" + shop.String() + "/orders" + rest
}

// review sends a review body for the order and returns the raw answer.
func (f fixture) review(c apptest.Client, id uuid.UUID, body string) *httptest.ResponseRecorder {
	return c.Call(http.MethodPost, f.consolePath(f.shop, "/"+id.String()+"/review"), body)
}

func version(v int, rest string) string {
	if rest != "" {
		rest = "," + rest
	}
	return fmt.Sprintf(`{"version":%d%s}`, v, rest)
}

func (f fixture) booked(slot uuid.UUID) int {
	return f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slot)
}

// timers counts the scheduled deadlines of an order by kind and revision.
func (f fixture) timers(id uuid.UUID, kind string, revision int) int {
	return f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'timer.schedule' AND entity_id = $1
		AND payload->>'kind' = $2 AND (payload->>'revisionNo')::int = $3`, id.String(), kind, revision)
}

func TestReview_Approve(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	staff := f.staff("asha", "SHOP_STAFF")

	w := f.review(staff, x.o.ID, version(1, `"action":"APPROVE"`))
	if w.Code != http.StatusOK {
		t.Fatalf("approve = %d %s", w.Code, w.Body)
	}
	got := apptest.Decode[order.Detail](t, w)
	if got.Status != enum.OrderStatusAwaitingPayment || got.Version != 2 || got.Totals.TotalCents != x.o.Totals.TotalCents {
		t.Fatalf("approved = %+v", got)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND frozen_at IS NOT NULL AND decided_at IS NOT NULL`, x.o.ID) != 1 {
		t.Error("the totals were not frozen")
	}
	if f.timers(x.o.ID, "payment", 0) != 1 || f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.approved' AND entity_id = $1`, x.o.ID.String()) != 1 {
		t.Error("the payment timer or the order.approved event is missing")
	}
	if f.booked(x.slot) != 1 {
		t.Error("approving must keep the slot place")
	}
	if cust := f.get(f.me, x.o.ID); cust.Status != enum.OrderStatusAwaitingPayment || cust.ExpiresAt == nil {
		t.Errorf("customer view = %+v", cust)
	}

	// BIZ-17: a second review with the version of the first is stale.
	if w := f.review(staff, x.o.ID, version(1, `"action":"REJECT","reason":"changed my mind"`)); w.Code != http.StatusConflict ||
		apptest.ErrorOf(t, w).Error.Code != "STALE_ORDER" {
		t.Errorf("same version again = %d %s, want 409 STALE_ORDER", w.Code, w.Body)
	}
	// With the right version, the shop still cannot review a bill it approved.
	if w := f.review(staff, x.o.ID, version(2, `"action":"REJECT","reason":"changed my mind"`)); w.Code != http.StatusConflict ||
		apptest.ErrorOf(t, w).Error.Code != "INVALID_TRANSITION" {
		t.Errorf("reject after approve = %d %s, want 409 INVALID_TRANSITION", w.Code, w.Body)
	}
}

// BIZ-17: two shop members approve the same bill at once; one wins.
func TestReview_RaceHasOneWinner(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	a, b := f.staff("asha", "SHOP_STAFF"), f.staff("bimal", "SHOP_OWNER")
	codes := make(chan int, 2)
	for _, c := range []apptest.Client{a, b} {
		go func() { codes <- f.review(c, x.o.ID, version(1, `"action":"APPROVE"`)).Code }()
	}
	got := []int{<-codes, <-codes}
	if !(got[0] == 200 && got[1] == 409 || got[0] == 409 && got[1] == 200) {
		t.Errorf("answers = %v, want one 200 and one 409", got)
	}
	if f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.approved'`) != 1 || f.timers(x.o.ID, "payment", 0) != 1 {
		t.Error("the approval ran twice")
	}
}

func TestReview_Reject(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	owner := f.staff("asha", "SHOP_OWNER")

	for name, body := range map[string]string{
		"no reason":    version(1, `"action":"REJECT"`),
		"short reason": version(1, `"action":"REJECT","reason":"no"`),
		"with changes": version(1, `"action":"REJECT","reason":"sold out","changes":[{"itemId":"`+x.rice0.ID.String()+`","remove":true}]`),
		"with a slot":  version(1, `"action":"REJECT","reason":"sold out","slotId":"`+x.slot.String()+`"`),
		"no version":   `{"action":"REJECT","reason":"sold out"}`,
		"bad action":   version(1, `"action":"SHRUG","reason":"sold out"`),
		"no action":    version(1, `"reason":"sold out"`),
		"extra field":  version(1, `"action":"REJECT","reason":"sold out","price":1`),
	} {
		if w := f.review(owner, x.o.ID, body); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d %s, want 400", name, w.Code, w.Body)
		}
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status = 'SUBMITTED' AND version = 1`, x.o.ID) != 1 {
		t.Fatal("a refused review changed the order")
	}
	w := f.review(owner, x.o.ID, version(1, `"action":"REJECT","reason":"out of stock"`))
	if w.Code != http.StatusOK || apptest.Decode[order.Detail](t, w).Status != enum.OrderStatusRejected {
		t.Fatalf("reject = %d %s", w.Code, w.Body)
	}
	if f.booked(x.slot) != 0 || f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.rejected'`) != 1 {
		t.Error("rejecting must release the place and write order.rejected")
	}
	if got := f.get(f.me, x.o.ID); len(got.Actions) != 0 || got.ExpiresAt != nil {
		t.Errorf("customer view of a rejected order = %+v", got)
	}
}

// Only the order's own shop reviews it: another shop's member, a driver and a
// customer get "not found" or "forbidden", never a change.
func TestReview_WhoMay(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	otherShop, _ := f.readyShop("other")
	theirs := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "theirs", Email: "theirs@example.com", Name: "Theirs"}))
	f.a.Grant(theirs.Session.Me.ID, &otherShop, "SHOP_OWNER")
	driver := f.staff("driver", "SHOP_DRIVER")
	body := version(1, `"action":"APPROVE"`)

	if w := theirs.Call(http.MethodPost, f.consolePath(f.shop, "/"+x.o.ID.String()+"/review"), body); w.Code != http.StatusNotFound {
		t.Errorf("another shop's owner on this shop = %d, want 404", w.Code)
	}
	if w := theirs.Call(http.MethodPost, f.consolePath(otherShop, "/"+x.o.ID.String()+"/review"), body); w.Code != http.StatusNotFound {
		t.Errorf("another shop's owner on their own shop with this order = %d, want 404", w.Code)
	}
	if w := driver.Call(http.MethodPost, f.consolePath(f.shop, "/"+x.o.ID.String()+"/review"), body); w.Code != http.StatusForbidden && w.Code != http.StatusNotFound {
		t.Errorf("a shop driver = %d, want 403 or 404", w.Code)
	}
	if w := f.review(f.me, x.o.ID, body); w.Code != http.StatusNotFound {
		t.Errorf("a customer on the console = %d, want 404", w.Code)
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status = 'SUBMITTED'`, x.o.ID) != 1 {
		t.Fatal("somebody who may not review changed the order")
	}
}
