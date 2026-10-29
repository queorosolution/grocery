package order_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/test/apptest"
)

func (f fixture) report(c apptest.Client, id uuid.UUID, body string) *http.Response {
	return c.Call(http.MethodPost, "/orders/"+id.String()+"/tickets", body).Result()
}

// FR-RFD-05: a paid order can be reported; the ticket reaches the admin queue and is resolved there.
func TestTicket_ReportAndResolve(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	tk := apptest.Decode[order.Ticket](t, f.me.CallWith(http.MethodPost, "/orders/"+s.id.String()+"/tickets", `{"reason":"MISSING_ITEM","note":"No sugar in the bag"}`, nil))
	if tk.Status != "OPEN" || tk.Reason != order.ReasonMissingItem || tk.OrderID != s.id || tk.ShopID != f.shop || tk.OrderCode == "" || tk.Note == nil {
		t.Fatalf("ticket = %+v", tk)
	}
	// The ticket belongs to the customer who is signed in, not to what the body says.
	if f.a.Count(`SELECT count(*) FROM support_tickets WHERE id = $1 AND user_id = $2`, tk.ID, f.me.Session.Me.ID) != 1 {
		t.Error("the ticket is not the customer's")
	}

	admin := f.admin()
	open := apptest.Decode[struct{ Data []order.Ticket }](t, admin.OK(http.MethodGet, "/admin/tickets?status=OPEN", ""))
	if len(open.Data) != 1 || open.Data[0].ID != tk.ID {
		t.Fatalf("queue = %+v", open.Data)
	}
	admin.Expect(http.MethodPost, "/admin/tickets/"+tk.ID.String()+"/resolve", `{"resolution":"x"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	done := apptest.Decode[order.Ticket](t, admin.OK(http.MethodPost, "/admin/tickets/"+tk.ID.String()+"/resolve", `{"resolution":"Sent the sugar with the next order"}`))
	if done.Status != "RESOLVED" || done.ResolvedAt == nil || done.Resolution == nil {
		t.Fatalf("resolved = %+v", done)
	}
	admin.Expect(http.MethodPost, "/admin/tickets/"+tk.ID.String()+"/resolve", `{"resolution":"Again please"}`, http.StatusConflict, "CONFLICT")
	if n := len(apptest.Decode[struct{ Data []order.Ticket }](t, admin.OK(http.MethodGet, "/admin/tickets?status=OPEN", "")).Data); n != 0 {
		t.Errorf("%d tickets still open", n)
	}
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'ticket.resolve'`) != 1 {
		t.Error("the resolution is not audited")
	}
	admin.Expect(http.MethodGet, "/admin/tickets?status=NOPE", "", http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestTicket_Rules(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	if r := f.report(f.me, x.o.ID, `{"reason":"LATE"}`); r.StatusCode != http.StatusConflict {
		t.Errorf("before payment: %d, want 409", r.StatusCode)
	}
	f.paid(x)
	for _, body := range []string{`{"reason":"NOPE"}`, `{}`, `{"reason":"OTHER","note":"` + string(make([]byte, 0)) + longNote() + `"}`} {
		if r := f.report(f.me, x.o.ID, body); r.StatusCode != http.StatusBadRequest {
			t.Errorf("%.30s: %d, want 400", body, r.StatusCode)
		}
	}
	// Another customer, or a shop member, does not find the order.
	stranger := f.a.As(f.a.SignedIn(claims("someone")))
	if r := f.report(stranger, x.o.ID, `{"reason":"LATE"}`); r.StatusCode != http.StatusNotFound {
		t.Errorf("another customer: %d, want 404", r.StatusCode)
	}
	if r := f.report(f.me, uuid.New(), `{"reason":"LATE"}`); r.StatusCode != http.StatusNotFound {
		t.Errorf("unknown order: %d, want 404", r.StatusCode)
	}
	// The queue is the admin's only.
	staff := f.staff("ravi", "SHOP_STAFF")
	for _, c := range []apptest.Client{f.me, staff} {
		c.Expect(http.MethodGet, "/admin/tickets", "", http.StatusForbidden, "FORBIDDEN")
	}
	// Ten a day, then slow down.
	for range 10 {
		f.report(f.me, x.o.ID, `{"reason":"OTHER"}`)
	}
	if r := f.report(f.me, x.o.ID, `{"reason":"OTHER"}`); r.StatusCode != http.StatusTooManyRequests {
		t.Errorf("11th ticket: %d, want 429", r.StatusCode)
	}
}

func longNote() string {
	b := make([]byte, 501)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
