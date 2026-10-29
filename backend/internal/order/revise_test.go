package order_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

func changes(cs ...string) string {
	out := `"action":"REVISE","changes":[`
	for i, c := range cs {
		if i > 0 {
			out += ","
		}
		out += c
	}
	return out + "]"
}

func qtyOf(id uuid.UUID, qty int) string {
	return `{"itemId":"` + id.String() + `","qty":` + strconv.Itoa(qty) + `}`
}

func removeOf(id uuid.UUID) string { return `{"itemId":"` + id.String() + `","remove":true}` }

func substituteOf(id, product uuid.UUID, qty int) string {
	return `{"itemId":"` + id.String() + `","substitute":{"shopProductId":"` + product.String() + `","qty":` + strconv.Itoa(qty) + `}}`
}

func linesByName(d order.Detail) map[string]order.LineView {
	out := map[string]order.LineView{}
	for _, l := range d.Lines {
		out[l.Name] = l
	}
	return out
}

func TestReview_Revise(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	staff := f.staff("asha", "SHOP_STAFF")

	// Rice goes to 2 packs, sugar is removed.
	w := f.review(staff, x.o.ID, version(1, changes(qtyOf(x.rice0.ID, 2), removeOf(x.sug0.ID))+`,"reason":"no sugar today"`))
	if w.Code != http.StatusOK {
		t.Fatalf("revise = %d %s", w.Code, w.Body)
	}
	got := apptest.Decode[order.Detail](t, w)
	if got.Status != enum.OrderStatusRevised || got.RevisionNo != 1 || got.Version != 2 {
		t.Fatalf("revised = %+v", got)
	}
	if got.Totals.ItemsCents != 2*itemPrice || got.Totals.TotalCents != 2*itemPrice+deliveryFee {
		t.Errorf("totals = %+v, want 2 rice and the delivery fee", got.Totals)
	}
	lines := linesByName(got)
	if len(got.Lines) != 2 || lines["Samba Rice 5kg"].Qty != 2 || lines["Samba Rice 5kg"].State != "ACTIVE" ||
		lines["White Sugar 1kg"].State != "REMOVED" || *lines["White Sugar 1kg"].LineCents != 0 {
		t.Errorf("lines = %+v", got.Lines)
	}
	if len(got.PreviousLines) != 2 {
		t.Errorf("previous lines = %+v, want the two of the first bill", got.PreviousLines)
	}
	for _, l := range got.PreviousLines {
		if l.RevisionNo != 0 || l.State != "ACTIVE" {
			t.Errorf("the first bill was changed: %+v", l)
		}
	}
	if f.timers(x.o.ID, "revision", 1) != 1 || f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.revised' AND payload->>'revisionNo' = '1'`) != 1 {
		t.Error("the revision timer or the order.revised event (with the new revision) is missing")
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND frozen_at IS NULL AND status_reason = 'no sugar today'`, x.o.ID) != 1 {
		t.Error("a revised bill must not be frozen yet, and the note is kept")
	}
	if f.booked(x.slot) != 1 {
		t.Error("the slot place must stay")
	}
	cust := f.get(f.me, x.o.ID)
	if cust.Totals.TotalCents != got.Totals.TotalCents || len(cust.Actions) != 2 || cust.Actions[0] != order.ActionAcceptRevision {
		t.Errorf("customer sees %+v", cust)
	}
	if cnt := apptest.Decode[order.SummaryPage](t, f.me.OK(http.MethodGet, "/orders", "")).Data[0].ItemCount; cnt != 1 {
		t.Errorf("item count = %d, want 1 (a removed line does not count)", cnt)
	}

	// The customer accepts. Another revision number is stale, the right one freezes the bill.
	f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":2,"revisionNo":0}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":2,"revisionNo":5}`, http.StatusConflict, "STALE_ORDER")
	f.me.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":1,"revisionNo":1}`, http.StatusConflict, "STALE_ORDER")
	staff.Expect(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":2,"revisionNo":1}`, http.StatusNotFound, "NOT_FOUND")
	done := apptest.Decode[order.Detail](t, f.me.OK(http.MethodPost, "/orders/"+x.o.ID.String()+"/accept-revision", `{"version":2,"revisionNo":1}`))
	if done.Status != enum.OrderStatusAwaitingPayment || done.Version != 3 {
		t.Fatalf("accepted = %+v", done)
	}
	if f.timers(x.o.ID, "payment", 1) != 1 || f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND frozen_at IS NOT NULL`, x.o.ID) != 1 {
		t.Error("accepting must freeze the bill and start the payment timer for revision 1")
	}
	// The shop cannot revise or approve a bill that the customer owns now.
	if w := f.review(staff, x.o.ID, version(3, `"action":"APPROVE"`)); w.Code != http.StatusConflict {
		t.Errorf("approve after accept = %d, want 409", w.Code)
	}
}

func TestReview_ReviseSubstitute(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	staff := f.staff("asha", "SHOP_OWNER")
	brown := f.product(f.shop, "Brown Sugar 1kg", 22000, 22500, true)

	w := f.review(staff, x.o.ID, version(1, changes(substituteOf(x.sug0.ID, brown, 2))))
	if w.Code != http.StatusOK {
		t.Fatalf("substitute = %d %s", w.Code, w.Body)
	}
	got := apptest.Decode[order.Detail](t, w)
	lines := linesByName(got)
	sub, gone := lines["Brown Sugar 1kg"], lines["White Sugar 1kg"]
	if sub.State != "SUBSTITUTE" || sub.Qty != 2 || *sub.UnitPriceCents != 22500 || gone.State != "REMOVED" || sub.ReplacesItemID == nil || *sub.ReplacesItemID != gone.ID {
		t.Fatalf("substitute lines = %+v", got.Lines)
	}
	if want := int64(itemPrice + 2*22500); int64(got.Totals.ItemsCents) != want {
		t.Errorf("items = %d, want %d: the substitute is priced by the server", got.Totals.ItemsCents, want)
	}
}

func TestReview_ReviseMovesTheSlot(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	staff := f.staff("asha", "SHOP_STAFF")
	next := f.slot(f.shop, 10)
	full := f.slot(f.shop, 1)
	f.a.Exec(`UPDATE slots SET booked = 1 WHERE id = $1`, full)
	otherShop, _ := f.readyShop("other")
	foreign := f.firstSlot(otherShop)

	change := qtyOf(x.rice0.ID, 2)
	for name, slot := range map[string]uuid.UUID{"a full slot": full, "another shop's slot": foreign} {
		if w := f.review(staff, x.o.ID, version(1, changes(change)+`,"slotId":"`+slot.String()+`"`)); w.Code < 400 {
			t.Errorf("%s was accepted: %d %s", name, w.Code, w.Body)
		}
	}
	if f.booked(x.slot) != 1 || f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status = 'SUBMITTED' AND version = 1 AND revision_no = 0`, x.o.ID) != 1 {
		t.Fatal("a refused revision changed the order or its slot")
	}
	if f.a.Count(`SELECT count(*) FROM order_items WHERE order_id = $1`, x.o.ID) != 2 {
		t.Fatal("a refused revision left lines behind")
	}
	if w := f.review(staff, x.o.ID, version(1, changes(change)+`,"slotId":"`+next.String()+`"`)); w.Code != http.StatusOK {
		t.Fatalf("new slot = %d %s", w.Code, w.Body)
	}
	if f.booked(x.slot) != 0 || f.booked(next) != 1 || f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND slot_id = $2`, x.o.ID, next) != 1 {
		t.Error("the new window must be booked and the old one released")
	}
}

func TestReview_ReviseRefusals(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	staff := f.staff("asha", "SHOP_STAFF")
	hidden := f.product(f.shop, "Hidden Tea", 1000, 1100, false)
	otherShop, _ := f.readyShop("other")
	theirs := f.product(otherShop, "Their Tea", 1000, 1100, true)
	rice, sugar := x.rice0.ID, x.sug0.ID

	cases := map[string]struct {
		body string
		code string
	}{
		"no changes":              {version(1, `"action":"REVISE"`), "VALIDATION_ERROR"},
		"empty changes":           {version(1, changes()), "VALIDATION_ERROR"},
		"same quantity":           {version(1, changes(`{"itemId":"`+rice.String()+`","qty":1}`)), "VALIDATION_ERROR"},
		"quantity zero":           {version(1, changes(`{"itemId":"`+rice.String()+`","qty":0}`)), "VALIDATION_ERROR"},
		"quantity too big":        {version(1, changes(`{"itemId":"`+rice.String()+`","qty":1000}`)), "VALIDATION_ERROR"},
		"two edits for one line":  {version(1, changes(`{"itemId":"`+rice.String()+`","qty":2,"remove":true}`)), "VALIDATION_ERROR"},
		"an edit that edits none": {version(1, changes(`{"itemId":"`+rice.String()+`"}`)), "VALIDATION_ERROR"},
		"a line twice":            {version(1, changes(qtyOf(rice, 2), removeOf(rice))), "VALIDATION_ERROR"},
		"an unknown line":         {version(1, changes(removeOf(uuid.New()))), "VALIDATION_ERROR"},
		"every line removed":      {version(1, changes(removeOf(rice), removeOf(sugar))), "VALIDATION_ERROR"},
		"a price in the change":   {version(1, changes(`{"itemId":"`+rice.String()+`","qty":2,"unitPriceCents":1}`)), "VALIDATION_ERROR"},
		"an unavailable product":  {version(1, changes(substituteOf(sugar, hidden, 1))), "ITEM_UNAVAILABLE"},
		"another shop's product":  {version(1, changes(substituteOf(sugar, theirs, 1))), "ITEM_UNAVAILABLE"},
		"the same product again":  {version(1, changes(substituteOf(sugar, x.rice, 1))), "VALIDATION_ERROR"},
		"over the pack limit":     {version(1, changes(`{"itemId":"`+rice.String()+`","qty":99}`)), "QTY_LIMIT"},
		"a slot with no slot":     {version(1, changes(qtyOf(rice, 2))+`,"slotId":"not-a-uuid"`), "VALIDATION_ERROR"},
	}
	for name, c := range cases {
		w := f.review(staff, x.o.ID, c.body)
		if w.Code < 400 || apptest.ErrorOf(t, w).Error.Code != c.code {
			t.Errorf("%s = %d %s, want %s", name, w.Code, w.Body, c.code)
		}
	}
	if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND status = 'SUBMITTED' AND version = 1 AND revision_no = 0`, x.o.ID) != 1 ||
		f.a.Count(`SELECT count(*) FROM order_items WHERE order_id = $1`, x.o.ID) != 2 {
		t.Fatal("a refused revision left something behind")
	}
}
