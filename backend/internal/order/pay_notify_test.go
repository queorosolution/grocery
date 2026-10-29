package order_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"gp/internal/events"
)

// FR-PAY-04: a signed success callback pays the order, in one step: the payment,
// the move, the ledger and the event.
func TestPayHere_Notify_Success(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.attempt(x)
	w := f.notify(callback(c, "2"))
	if w.Code != http.StatusOK {
		t.Fatalf("notify = %d %s", w.Code, w.Body)
	}
	if got := f.status(x.o.ID); got != "PAID" {
		t.Fatalf("order = %s, want PAID", got)
	}
	if f.payments(x.o.ID, "SUCCESS") != 1 || f.a.Count(`SELECT count(*) FROM payments WHERE order_id = $1 AND provider_ref = $2 AND method = 'VISA'`, x.o.ID, payhereRef) != 1 {
		t.Error("the payment is not SUCCESS with PayHere's reference and method")
	}
	if f.events("order.paid", x.o.ID) != 1 || f.a.Count(`SELECT count(*) FROM order_events WHERE order_id = $1 AND to_status = 'PAID' AND actor_role = 'SYSTEM'`, x.o.ID) != 1 {
		t.Error("the move to PAID is not recorded once, by the system")
	}
	if f.ledger(x.o.ID) == 0 || f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND paid_at IS NOT NULL`, x.o.ID) != 1 {
		t.Error("no ledger rows, or paid_at is not set")
	}
	if p := f.get(f.me, x.o.ID).Payment; p == nil || p.Status == nil || *p.Status != "SUCCESS" {
		t.Error("the customer does not see the payment as SUCCESS")
	}
}

// BIZ-07: a forged callback changes nothing.
func TestPayHere_Notify_BadSignature(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.attempt(x)
	forged := callback(c, "2")
	forged.Set("md5sig", strings.Repeat("A", 32))
	if w := f.notify(forged); w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "sig") {
		t.Errorf("forged = %d %s, want a bare 400", w.Code, w.Body)
	}
	// Right signature, but over another status than the one it claims.
	tampered := callback(c, "-2")
	tampered.Set("status_code", "2")
	if w := f.notify(tampered); w.Code != http.StatusBadRequest {
		t.Errorf("tampered status = %d, want 400", w.Code)
	}
	if f.status(x.o.ID) != "AWAITING_PAYMENT" || f.payments(x.o.ID, "INITIATED") != 1 || f.ledger(x.o.ID) != 0 {
		t.Error("a forged callback changed something")
	}
}

// BIZ-06: a signed callback for another amount, merchant or currency pays nothing.
func TestPayHere_Notify_WrongAmount(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.attempt(x)
	id := c.Fields["order_id"]
	for name, v := range map[string]url.Values{
		"cheaper":  signed(id, "1.00", "2"),
		"dearer":   signed(id, "999999.00", "2"),
		"no cents": signed(id, strings.TrimSuffix(c.Fields["amount"], ".00"), "2"),
	} {
		if w := f.notify(v); w.Code != http.StatusOK {
			t.Errorf("%s: %d %s, want 200 (nothing to retry)", name, w.Code, w.Body)
		}
	}
	other := signed(id, c.Fields["amount"], "2")
	other.Set("merchant_id", "9999999")
	other.Set("md5sig", signSig("9999999", id, c.Fields["amount"], "LKR", "2"))
	usd := signed(id, c.Fields["amount"], "2")
	usd.Set("payhere_currency", "USD")
	usd.Set("md5sig", signSig(merchantID, id, c.Fields["amount"], "USD", "2"))
	for name, v := range map[string]url.Values{"another merchant": other, "another currency": usd} {
		if w := f.notify(v); w.Code != http.StatusOK {
			t.Errorf("%s: %d %s", name, w.Code, w.Body)
		}
	}
	if f.status(x.o.ID) != "AWAITING_PAYMENT" || f.payments(x.o.ID, "INITIATED") != 1 || f.ledger(x.o.ID) != 0 {
		t.Error("a callback that did not match the payment paid the order")
	}
}

// BIZ-08: the same callback again is a no-op.
func TestPayHere_Notify_Replay(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.paid(x)
	rows, versionAfter := f.ledger(x.o.ID), f.a.Count(`SELECT version FROM orders WHERE id = $1`, x.o.ID)
	for i := 0; i < 3; i++ {
		if w := f.notify(callback(c, "2")); w.Code != http.StatusOK {
			t.Fatalf("replay %d = %d %s", i, w.Code, w.Body)
		}
	}
	if f.payments(x.o.ID, "SUCCESS") != 1 || f.ledger(x.o.ID) != rows || f.events("order.paid", x.o.ID) != 1 ||
		f.a.Count(`SELECT version FROM orders WHERE id = $1`, x.o.ID) != versionAfter || f.a.Count(`SELECT count(*) FROM refunds WHERE order_id = $1`, x.o.ID) != 0 {
		t.Error("a replayed callback was not a no-op")
	}
}

// BIZ-09: money that comes after the order closed is never lost and never pays it.
func TestPayHere_Notify_Late(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.attempt(x)
	if moved, err := f.a.Orders.Expire(t.Context(), x.o.ID, events.TimerPayment, x.o.RevisionNo); err != nil || !moved {
		t.Fatalf("expire = %v, %v", moved, err)
	}
	if w := f.notify(callback(c, "2")); w.Code != http.StatusOK {
		t.Fatalf("late notify = %d %s", w.Code, w.Body)
	}
	if got := f.status(x.o.ID); got != "EXPIRED" {
		t.Errorf("order = %s, want EXPIRED", got)
	}
	if f.payments(x.o.ID, "SUCCESS") != 1 || f.ledger(x.o.ID) != 0 {
		t.Error("the late money must be recorded as SUCCESS and post no ledger rows")
	}
	if f.a.Count(`SELECT count(*) FROM refunds WHERE order_id = $1 AND type = 'LATE_PAYMENT' AND status = 'REQUESTED'
		AND amount_cents = (SELECT amount_cents FROM payments WHERE order_id = $1) AND payment_id IS NOT NULL`, x.o.ID) != 1 {
		t.Error("no LATE_PAYMENT refund for the amount that arrived")
	}
	if f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'refund.requested' AND shop_id = $1`, f.shop) != 1 {
		t.Error("the admin is not told")
	}
	f.notify(callback(c, "2")) // and a replay does not ask twice
	if f.a.Count(`SELECT count(*) FROM refunds WHERE order_id = $1`, x.o.ID) != 1 {
		t.Error("a replay of a late payment opened another refund")
	}
}

// If the total changed after the attempt started, the money does not match the
// order: it is returned and the order stays unpaid.
func TestPayHere_Notify_TotalChanged(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.attempt(x)
	f.makeCoupon(f.admin(), "SAVE10")
	if w := f.apply(f.me, x.o.ID, x.o.Version, "SAVE10"); w.Code != http.StatusOK {
		t.Fatalf("apply = %d %s", w.Code, w.Body)
	}
	if w := f.notify(callback(c, "2")); w.Code != http.StatusOK {
		t.Fatalf("notify = %d %s", w.Code, w.Body)
	}
	if f.status(x.o.ID) != "AWAITING_PAYMENT" || f.ledger(x.o.ID) != 0 ||
		f.a.Count(`SELECT count(*) FROM refunds WHERE order_id = $1 AND type = 'FULL' AND status = 'REQUESTED'`, x.o.ID) != 1 {
		t.Error("an old amount paid a changed order, or the money was not returned")
	}
}

// DATA-03: card data is never stored.
func TestPayHere_Notify_CardFieldsStripped(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.attempt(x)
	v := callback(c, "2", "card_holder_name", "Nimal Perera", "card_no", "************1234", "card_expiry", "12/30", "card_pan", "4111111111111111", "custom_1", "keep")
	if w := f.notify(v); w.Code != http.StatusOK {
		t.Fatalf("notify = %d %s", w.Code, w.Body)
	}
	for _, table := range []string{"payments", "audit_log", "outbox", "order_events", "refunds"} {
		for _, secret := range []string{"Nimal Perera", "12/30", "4111111111111111", v.Get("md5sig"), "************1234"} {
			if n := f.a.Count(`SELECT count(*) FROM `+table+` t WHERE t::text LIKE '%' || $1 || '%'`, secret); n != 0 {
				t.Errorf("%s holds %q", table, secret)
			}
		}
	}
	if f.a.Count(`SELECT count(*) FROM payments WHERE raw ? 'card_no' OR raw ? 'card_holder_name' OR raw ? 'card_expiry' OR raw ? 'md5sig' OR raw ? 'card_pan'`) != 0 {
		t.Error("payments.raw has card fields or the signature")
	}
	if f.a.Count(`SELECT count(*) FROM payments WHERE raw->>'custom_1' = 'keep' AND raw->>'status_code' = '2'`) != 1 {
		t.Error("payments.raw lost the fields that are safe to keep")
	}
}

// The other status codes of docs/09 section 2.3.
func TestPayHere_Notify_OtherCodes(t *testing.T) {
	for name, tc := range map[string]struct {
		code, payment string
	}{
		"pending":   {"0", "INITIATED"},
		"cancelled": {"-1", "CANCELLED"},
		"failed":    {"-2", "FAILED"},
	} {
		f := newFixture(t)
		x := f.unpaid()
		c := f.attempt(x)
		if w := f.notify(callback(c, tc.code)); w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", name, w.Code, w.Body)
		}
		if f.payments(x.o.ID, tc.payment) != 1 || f.status(x.o.ID) != "AWAITING_PAYMENT" {
			t.Errorf("%s: payment is not %s, or the order moved", name, tc.payment)
		}
		// A failed attempt does not stop the next one.
		if tc.code != "0" {
			if w := f.notify(callback(c, "2")); w.Code != http.StatusOK {
				t.Fatalf("%s then success: %d", name, w.Code)
			}
		}
	}
}

func TestPayHere_Notify_Chargeback(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.paid(x)
	rows := f.ledger(x.o.ID)
	if w := f.notify(callback(c, "-3")); w.Code != http.StatusOK {
		t.Fatalf("chargeback = %d %s", w.Code, w.Body)
	}
	if f.payments(x.o.ID, "CHARGEBACK") != 1 || f.ledger(x.o.ID) != rows || f.status(x.o.ID) != "PAID" {
		t.Error("a chargeback must mark the payment and leave the order and the ledger for the admin")
	}
	// A late failure never undoes a payment.
	f.notify(callback(c, "-2"))
	if f.payments(x.o.ID, "CHARGEBACK") != 1 {
		t.Error("a failure code changed a charged-back payment")
	}
}

func TestPayHere_Notify_Shape(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	c := f.attempt(x)
	for name, drop := range map[string]string{"merchant": "merchant_id", "order": "order_id", "payment id": "payment_id", "amount": "payhere_amount",
		"currency": "payhere_currency", "status": "status_code", "signature": "md5sig"} {
		v := callback(c, "2")
		v.Del(drop)
		if w := f.notify(v); w.Code != http.StatusBadRequest {
			t.Errorf("without %s = %d, want 400", name, w.Code)
		}
	}
	for name, id := range map[string]string{"unknown payment": "5f2c1c6e-5a2b-4a37-9a67-2a1f3c6d8e90", "not an id": "pay_123"} {
		if w := f.notify(signed(id, "10.00", "2")); w.Code != http.StatusOK {
			t.Errorf("%s = %d %s, want 200 and ignored", name, w.Code, w.Body)
		}
	}
	if w := f.a.Call(http.MethodPost, "/v1/webhooks/payhere", `{"order_id":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("a JSON body = %d, want 400", w.Code)
	}
	big := url.Values{"x": {strings.Repeat("a", 20<<10)}}
	if w := f.notify(big); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("a 20 KB body = %d, want 413", w.Code)
	}
	if f.status(x.o.ID) != "AWAITING_PAYMENT" || f.ledger(x.o.ID) != 0 {
		t.Error("a bad callback changed the order")
	}
}
