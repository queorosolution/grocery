package order_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/internal/refund"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

const refundReason = "Customer called to cancel"

func (f fixture) refundPath(id uuid.UUID) string {
	return "/console/shops/" + f.shop.String() + "/orders/" + id.String() + "/refund-requests"
}

func (f fixture) fullRequest(c apptest.Client, id uuid.UUID) refund.Refund {
	f.a.T.Helper()
	return apptest.Decode[refund.Refund](f.a.T, c.OK(http.MethodPost, f.refundPath(id), `{"type":"FULL","reason":"`+refundReason+`"}`))
}

func (f fixture) partialRequest(c apptest.Client, id, item uuid.UUID) refund.Refund {
	f.a.T.Helper()
	return apptest.Decode[refund.Refund](f.a.T, c.OK(http.MethodPost, f.refundPath(id), `{"type":"PARTIAL","orderItemId":"`+item.String()+`","reason":"Out of stock"}`))
}

func (f fixture) decide(id uuid.UUID, approve bool, note string) *http.Response {
	body := `{"approve":` + map[bool]string{true: "true", false: "false"}[approve] + `,"note":"` + note + `"}`
	return f.admin().Call(http.MethodPost, "/admin/refunds/"+id.String()+"/decide", body).Result()
}

func (f fixture) paidOut(id uuid.UUID, ref string) *http.Response {
	return f.admin().Call(http.MethodPost, "/admin/refunds/"+id.String()+"/paid-out", `{"reference":"`+ref+`"}`).Result()
}

func (f fixture) refundStatus(id uuid.UUID) string {
	f.a.T.Helper()
	var s string
	if err := f.a.Env.Owner.QueryRow(f.a.T.Context(), `SELECT status FROM refunds WHERE id = $1`, id).Scan(&s); err != nil {
		f.a.T.Fatal(err)
	}
	return s
}

func (f fixture) ledgerSum(id uuid.UUID) (sum int64) {
	for _, r := range f.ledgerOf(id) {
		sum += r.Amount
	}
	return sum
}

// FR-RFD-02 / FR-RFD-03 / FR-FIN-02: the shop asks, the admin approves (T19), the money
// goes back, and only then do the ledger rows reverse.
func TestRefund_FullCancelPath(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	total := int64(f.a.Count(`SELECT total_cents FROM orders WHERE id = $1`, s.id))
	before := f.ledger(s.id)

	r := f.fullRequest(s.staff, s.id)
	if r.Status != enum.RefundStatusRequested || r.Type != enum.RefundTypeFull || int64(r.AmountCents) != total || r.RequestedBy == nil {
		t.Fatalf("request = %+v", r)
	}
	if f.events("refund.requested", r.ID) != 1 || f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'refund.request'`) != 1 {
		t.Error("the request is not announced and audited")
	}
	// One request at a time: a second full cancel, or a partial, is a conflict.
	s.staff.Expect(http.MethodPost, f.refundPath(s.id), `{"type":"FULL","reason":"again please"}`, http.StatusConflict, "CONFLICT")
	if f.ledger(s.id) != before || f.status(s.id) != "PAID" {
		t.Fatal("a request alone changed the order or the ledger")
	}

	// Approving is T19.
	if resp := f.decide(r.ID, true, "Shop cannot deliver"); resp.StatusCode != http.StatusOK {
		t.Fatalf("decide = %d", resp.StatusCode)
	}
	if f.status(s.id) != "CANCELLED_REFUND" || f.refundStatus(r.ID) != "APPROVED" {
		t.Fatalf("order %s, refund %s", f.status(s.id), f.refundStatus(r.ID))
	}
	if f.booked(s.x.slot) != 0 || f.events("order.cancelled_refund", s.id) != 1 {
		t.Error("T19 did not release the slot or emit its event")
	}
	if f.ledger(s.id) != before {
		t.Error("the ledger changed before the money went back")
	}
	if resp := f.decide(r.ID, true, ""); resp.StatusCode != http.StatusConflict {
		t.Errorf("deciding twice = %d, want 409", resp.StatusCode)
	}

	// Paid out: the ledger takes everything back, so the order adds up to 0.
	if resp := f.paidOut(r.ID, "BANK-REF-77"); resp.StatusCode != http.StatusOK {
		t.Fatalf("paid-out = %d", resp.StatusCode)
	}
	if f.refundStatus(r.ID) != "PAID_OUT" || f.ledgerSum(s.id) != 0 {
		t.Errorf("refund %s, ledger sum %d, want PAID_OUT and 0", f.refundStatus(r.ID), f.ledgerSum(s.id))
	}
	if f.a.Count(`SELECT count(*) FROM ledger_entries WHERE refund_id = $1 AND type = 'REFUND' AND amount_cents < 0`, r.ID) != 2 {
		t.Error("want a REFUND row for the shop and one for the platform")
	}
	if f.paidOut(r.ID, "BANK-REF-77").StatusCode != http.StatusConflict {
		t.Error("paying out twice is not a conflict")
	}
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE action IN ('refund.decide', 'refund.paid_out', 'order.transition')`) < 3 {
		t.Error("the decision, the payout and the move are not all audited")
	}
}

func TestRefund_RejectKeepsOrderAndLedger(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	r := f.fullRequest(s.staff, s.id)
	before := f.ledger(s.id)
	if resp := f.decide(r.ID, false, "Not allowed"); resp.StatusCode != http.StatusOK {
		t.Fatalf("reject = %d", resp.StatusCode)
	}
	if f.refundStatus(r.ID) != "REJECTED" || f.status(s.id) != "PAID" || f.ledger(s.id) != before {
		t.Error("a rejected request changed the order or the ledger")
	}
	if f.paidOut(r.ID, "BANK-REF-78").StatusCode != http.StatusConflict {
		t.Error("a rejected refund can be paid out")
	}
	f.fullRequest(s.staff, s.id) // after a rejection the shop may ask again
}

func TestRefund_Partial(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	s.staff.Expect(http.MethodPost, f.refundPath(s.id), `{"type":"PARTIAL","orderItemId":"`+s.x.rice0.ID.String()+`","reason":"Out of stock"}`, http.StatusConflict, "INVALID_TRANSITION")
	f.move(s.staff, s.id, "PICKING")
	v := f.get(f.me, s.id).Version

	r := f.partialRequest(s.staff, s.id, s.x.rice0.ID)
	if r.Type != enum.RefundTypePartial || int64(r.AmountCents) != itemPrice {
		t.Fatalf("partial = %+v", r)
	}
	got := f.get(f.me, s.id)
	if got.Version != v+1 || lineState(got, s.x.rice0.ID) != "MISSING" {
		t.Errorf("version %d (was %d), line %s", got.Version, v, lineState(got, s.x.rice0.ID))
	}
	s.staff.Expect(http.MethodPost, f.refundPath(s.id), `{"type":"PARTIAL","orderItemId":"`+s.x.rice0.ID.String()+`","reason":"Out of stock"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	s.staff.Expect(http.MethodPost, f.refundPath(s.id), `{"type":"FULL","reason":"cancel it all"}`, http.StatusConflict, "CONFLICT")

	f.decide(r.ID, true, "")
	if f.status(s.id) != "PICKING" || f.refundStatus(r.ID) != "APPROVED" {
		t.Fatalf("order %s, refund %s: a partial refund must not cancel the order", f.status(s.id), f.refundStatus(r.ID))
	}
	f.paidOut(r.ID, "BANK-REF-79")
	var shop, platform int64
	for _, row := range f.ledgerOf(s.id) {
		if row.Type == "REFUND" && row.Party == "SHOP" {
			shop = row.Amount
		}
		if row.Type == "REFUND" && row.Party == "PLATFORM" {
			platform = row.Amount
		}
	}
	if shop != -itemShop || platform != -(itemPrice-itemShop) {
		t.Errorf("REFUND rows: shop %d, platform %d; want %d and %d", shop, platform, -itemShop, -(itemPrice - itemShop))
	}
}

func lineState(d order.Detail, item uuid.UUID) string {
	for _, l := range d.Lines {
		if l.ID == item {
			return l.State
		}
	}
	return ""
}

func TestRefund_RejectedPartialPutsTheLineBack(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	f.move(s.staff, s.id, "PICKING")
	r := f.partialRequest(s.staff, s.id, s.x.sug0.ID)
	f.decide(r.ID, false, "Check the shelf again")
	if lineState(f.get(f.me, s.id), s.x.sug0.ID) != "ACTIVE" {
		t.Error("the line is still missing after the rejection")
	}
	f.partialRequest(s.staff, s.id, s.x.sug0.ID) // and it can be asked again
}

// The admin cancels on their own (T19): the refund record is made for them.
func TestRefund_AdminCancelMakesTheRefund(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	total := int64(f.a.Count(`SELECT total_cents FROM orders WHERE id = $1`, s.id))
	v := f.get(f.me, s.id).Version
	a := f.admin()
	a.Expect(http.MethodPost, "/admin/orders/"+s.id.String()+"/transition", version(v, `"to":"CANCELLED_REFUND"`), http.StatusBadRequest, "VALIDATION_ERROR")
	a.OK(http.MethodPost, "/admin/orders/"+s.id.String()+"/transition", version(v, `"to":"CANCELLED_REFUND","reason":"Shop burnt down"`))
	if f.status(s.id) != "CANCELLED_REFUND" {
		t.Fatalf("status = %s", f.status(s.id))
	}
	if f.a.Count(`SELECT count(*) FROM refunds WHERE order_id = $1 AND type = 'FULL' AND status = 'APPROVED' AND amount_cents = $2 AND payment_id IS NOT NULL`, s.id, total) != 1 {
		t.Error("no approved full refund of what the customer paid")
	}
}

func TestRefund_AdminCancelWaitsForOpenRequests(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	f.move(s.staff, s.id, "PICKING")
	f.partialRequest(s.staff, s.id, s.x.rice0.ID)
	v := f.get(f.me, s.id).Version
	f.admin().Expect(http.MethodPost, "/admin/orders/"+s.id.String()+"/transition", version(v, `"to":"CANCELLED_REFUND","reason":"Cancel it"`), http.StatusConflict, "CONFLICT")
	if f.status(s.id) != "PICKING" {
		t.Error("the order moved with a request still open")
	}
}

// If the driver already left, approving the cancel is refused and the admin rejects.
func TestRefund_ApproveAfterTheOrderLeft(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	r := f.fullRequest(s.staff, s.id)
	f.a.Exec(`UPDATE orders SET status = 'OUT_FOR_DELIVERY' WHERE id = $1`, s.id)
	if resp := f.decide(r.ID, true, ""); resp.StatusCode != http.StatusConflict {
		t.Fatalf("approve = %d, want 409", resp.StatusCode)
	}
	if f.refundStatus(r.ID) != "REQUESTED" || f.status(s.id) != "OUT_FOR_DELIVERY" {
		t.Error("a refused approval changed something")
	}
}

// Money that came in for an order that could not take it is approved and paid out
// without touching the order or the ledger (it never was in the ledger).
func TestRefund_LatePaymentTouchesNoLedger(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	id := uuid.New()
	f.a.Exec(`INSERT INTO refunds (id, order_id, shop_id, type, amount_cents, reason) VALUES ($1, $2, $3, 'LATE_PAYMENT', 5000, 'Paid twice')`, id, s.id, f.shop)
	before := f.ledger(s.id)
	f.decide(id, true, "")
	f.paidOut(id, "BANK-REF-80")
	if f.refundStatus(id) != "PAID_OUT" || f.status(s.id) != "PAID" || f.ledger(s.id) != before {
		t.Error("a late-payment refund changed the order or the ledger")
	}
}

func TestRefund_Authorisation(t *testing.T) {
	f := newFixture(t)
	s := f.paidFlow()
	// Drivers and customers cannot ask; staff of another shop do not find the order.
	s.driver.Expect(http.MethodPost, f.refundPath(s.id), `{"type":"FULL","reason":"cancel it"}`, http.StatusForbidden, "FORBIDDEN")
	f.me.Expect(http.MethodPost, f.refundPath(s.id), `{"type":"FULL","reason":"cancel it"}`, http.StatusNotFound, "NOT_FOUND")
	other, _ := f.readyShop("other")
	stranger := f.a.As(f.a.SignedIn(claims("stranger")))
	f.a.Grant(stranger.Session.Me.ID, &other, string(enum.RoleShopStaff))
	stranger.Expect(http.MethodPost, "/console/shops/"+other.String()+"/orders/"+s.id.String()+"/refund-requests", `{"type":"FULL","reason":"cancel it"}`, http.StatusNotFound, "NOT_FOUND")
	// Bad bodies.
	for _, body := range []string{`{"type":"LATE_PAYMENT","reason":"cancel it"}`, `{"type":"FULL","reason":"x"}`, `{"type":"FULL","orderItemId":"` + uuid.NewString() + `","reason":"cancel it"}`, `{"type":"PARTIAL","reason":"cancel it"}`} {
		s.staff.Expect(http.MethodPost, f.refundPath(s.id), body, http.StatusBadRequest, "VALIDATION_ERROR")
	}
	// Only the admin decides, lists and pays out.
	r := f.fullRequest(s.staff, s.id)
	for _, c := range []apptest.Client{s.staff, f.me, s.driver} {
		c.Expect(http.MethodPost, "/admin/refunds/"+r.ID.String()+"/decide", `{"approve":true}`, http.StatusForbidden, "FORBIDDEN")
		c.Expect(http.MethodGet, "/admin/refunds", "", http.StatusForbidden, "FORBIDDEN")
	}
	f.admin().Expect(http.MethodPost, "/admin/refunds/"+r.ID.String()+"/decide", `{"note":"no approve field"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	if f.refundStatus(r.ID) != "REQUESTED" {
		t.Error("a body without approve decided the refund")
	}
	list := apptest.Decode[struct{ Data []refund.Refund }](t, f.admin().OK(http.MethodGet, "/admin/refunds?status=REQUESTED", ""))
	if len(list.Data) != 1 || list.Data[0].ID != r.ID || list.Data[0].OrderCode == "" {
		t.Errorf("list = %+v", list.Data)
	}
	f.admin().Expect(http.MethodGet, "/admin/refunds?status=NOPE", "", http.StatusBadRequest, "VALIDATION_ERROR")
}
