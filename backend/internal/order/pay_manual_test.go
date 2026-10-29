package order_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"gp/internal/order"
	"gp/test/apptest"
)

func (f fixture) markPaid(c apptest.Client, id uuid.UUID, v int, ref string) (int, string) {
	w := c.Call(http.MethodPost, "/admin/orders/"+id.String()+"/mark-paid", `{"version":`+strconv.Itoa(v)+`,"reference":"`+ref+`"}`)
	return w.Code, w.Body.String()
}

// T10 in manual mode: the admin confirms the transfer; payment, move, ledger and audit are one step.
func TestMarkPaid_Manual(t *testing.T) {
	f := newFixture(t)
	x := f.awaiting()
	v := f.get(f.me, x.o.ID).Version
	admin := f.admin()

	if code, body := f.markPaid(admin, x.o.ID, v-1, "BANK-REF-001"); code != http.StatusConflict {
		t.Errorf("stale version = %d %s, want 409", code, body)
	}
	for _, bad := range []string{"", "ab", " padded ", "x"} {
		if code, _ := f.markPaid(admin, x.o.ID, v, bad); code != http.StatusBadRequest {
			t.Errorf("reference %q = %d, want 400", bad, code)
		}
	}
	for name, c := range map[string]apptest.Client{"customer": f.me, "shop owner": f.staff("asha", "SHOP_OWNER")} {
		if code, _ := f.markPaid(c, x.o.ID, v, "BANK-REF-001"); code != http.StatusForbidden {
			t.Errorf("%s = %d, want 403", name, code)
		}
	}
	if f.status(x.o.ID) != "AWAITING_PAYMENT" {
		t.Fatal("a refused call moved the order")
	}

	code, body := f.markPaid(admin, x.o.ID, v, "BANK-REF-001")
	if code != http.StatusOK {
		t.Fatalf("mark-paid = %d %s", code, body)
	}
	if d := f.get(f.me, x.o.ID); d.Status != "PAID" {
		t.Errorf("order = %s", d.Status)
	}
	if f.a.Count(`SELECT count(*) FROM payments WHERE order_id = $1 AND provider = 'MANUAL' AND status = 'SUCCESS' AND provider_ref = 'BANK-REF-001'
		AND amount_cents = (SELECT total_cents FROM orders WHERE id = $1)`, x.o.ID) != 1 {
		t.Error("no SUCCESS manual payment with the bank's reference and the order total")
	}
	if f.ledger(x.o.ID) == 0 || f.events("order.paid", x.o.ID) != 1 {
		t.Error("no ledger rows or no order.paid event")
	}
	if f.a.Count(`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action IN ('order.mark_paid', 'order.transition')`, x.o.ID.String()) != 2 {
		t.Error("the admin's move is not audited twice (mark_paid and transition)")
	}
	// Paid orders cannot be paid again.
	if code, _ := f.markPaid(admin, x.o.ID, v+1, "BANK-REF-002"); code != http.StatusConflict {
		t.Errorf("paying twice = %d, want 409", code)
	}
}

// A customer who asked for the bank details first: their open attempt is the one that is paid.
func TestMarkPaid_ClosesTheOpenAttempts(t *testing.T) {
	f := newFixture(t)
	x := f.awaiting()
	var acc string
	if err := f.a.Env.Owner.QueryRow(t.Context(), `SELECT id::text FROM payment_accounts WHERE provider = 'MANUAL'`).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	f.admin().OK(http.MethodPatch, "/admin/payment-accounts/"+acc, `{"bank":{"bankName":"Commercial Bank","accountName":"Krishna Super","accountNumber":"8001234567","branch":""}}`)
	d := f.get(f.me, x.o.ID)
	for _, key := range []string{"pay-key-0000000001", "pay-key-0000000002"} {
		if w := f.start(f.me, x.o.ID, d.Version, key); w.Code != http.StatusOK {
			t.Fatalf("start = %d %s", w.Code, w.Body)
		}
	}
	if code, body := f.markPaid(f.admin(), x.o.ID, d.Version, "BANK-REF-777"); code != http.StatusOK {
		t.Fatalf("mark-paid = %d %s", code, body)
	}
	if f.payments(x.o.ID, "SUCCESS") != 1 || f.payments(x.o.ID, "CANCELLED") != 1 || f.payments(x.o.ID, "INITIATED") != 0 {
		t.Error("the newest attempt should be SUCCESS and the other CANCELLED")
	}
}

func TestMarkPaid_Refusals(t *testing.T) {
	f := newFixture(t)
	admin := f.admin()
	// A bank reference belongs to one order.
	a := f.awaiting()
	av := f.get(f.me, a.o.ID).Version
	if code, body := f.markPaid(admin, a.o.ID, av, "BANK-REF-SAME"); code != http.StatusOK {
		t.Fatalf("first = %d %s", code, body)
	}
	b := f.secondAwaiting()
	if code, body := f.markPaid(admin, b.ID, b.Version, "BANK-REF-SAME"); code != http.StatusBadRequest {
		t.Errorf("a reused reference = %d %s, want 400", code, body)
	}
	if f.status(b.ID) != "AWAITING_PAYMENT" || f.payments(b.ID, "SUCCESS") != 0 {
		t.Error("a refused reference left a half-paid order")
	}
	// A PayHere order is paid by PayHere only.
	f.usePayHere()
	c := f.secondAwaiting()
	if code, _ := f.markPaid(admin, c.ID, c.Version, "BANK-REF-OTHER"); code != http.StatusConflict {
		t.Errorf("mark-paid on a PayHere order = %d, want 409", code)
	}
	if code, _ := f.markPaid(admin, uuid.New(), 1, "BANK-REF-NONE"); code != http.StatusNotFound {
		t.Errorf("unknown order = %d, want 404", code)
	}
}

// secondAwaiting is another approved order of the same customer and shop.
func (f fixture) secondAwaiting() order.Detail {
	f.a.T.Helper()
	n := uuid.NewString()
	rice := f.product(f.shop, "Red Rice "+n[:8], itemShop, itemPrice, true)
	f.cart(f.tenant, cartLine{rice, 1, itemPrice})
	o := f.placed(f.me, "key-"+n, f.body(`"slotId":"`+f.firstSlot(f.shop).String()+`"`, rice))
	if w := f.review(f.staff("staff"+n[:8], "SHOP_STAFF"), o.ID, version(1, `"action":"APPROVE"`)); w.Code != http.StatusOK {
		f.a.T.Fatalf("approve = %d %s", w.Code, w.Body)
	}
	return f.get(f.me, o.ID)
}
