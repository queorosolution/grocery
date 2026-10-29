package order_test

import (
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"gp/internal/payment/manual"
	"gp/internal/payment/payhere"
	"gp/internal/shared/apperr"
	"gp/test/apptest"
	"gp/test/fakes"
)

// FR-PAY-01: the checkout is built on the server, signed over the amount, and a
// retry with the same key is the same answer and not a second attempt.
func TestPay_Start_PayHere(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	w := f.start(f.me, x.o.ID, x.o.Version, payKey)
	if w.Code != http.StatusOK {
		t.Fatalf("start = %d %s", w.Code, w.Body)
	}
	c := apptest.Decode[payhere.Checkout](t, w)
	amount := f.totalOf(x.o.ID)
	if c.Provider != "PAYHERE" || c.Action != "https://sandbox.payhere.lk/pay/checkout" {
		t.Errorf("checkout = %s %s", c.Provider, c.Action)
	}
	want := map[string]string{
		"merchant_id": merchantID, "currency": "LKR", "amount": amount, "items": "Order " + x.o.Code,
		"return_url": apptest.WebOrigin + "/order/" + x.o.ID.String() + "?paid=1", "cancel_url": apptest.WebOrigin + "/order/" + x.o.ID.String(),
		"notify_url": "https://api.example.test/v1/webhooks/payhere", "first_name": "Nimal", "last_name": "-", "email": "nimal@example.com",
		"hash": payhere.CheckoutHash(merchantID, c.Fields["order_id"], amount, "LKR", merchantSecret),
	}
	for k, v := range want {
		if c.Fields[k] != v {
			t.Errorf("fields[%s] = %q, want %q", k, c.Fields[k], v)
		}
	}
	if n := f.payments(x.o.ID, "INITIATED"); n != 1 {
		t.Errorf("INITIATED payments = %d, want 1", n)
	}
	if f.a.Count(`SELECT count(*) FROM payments WHERE id::text = $1`, c.Fields["order_id"]) != 1 {
		t.Error("order_id is not the payment id")
	}
	if strings.Contains(w.Body.String(), merchantSecret) {
		t.Error("the merchant secret reached the browser")
	}

	again := f.start(f.me, x.o.ID, x.o.Version, payKey)
	if got := apptest.Decode[payhere.Checkout](t, again); again.Code != http.StatusOK || !reflect.DeepEqual(got, c) {
		t.Errorf("replay = %d %+v, want the first answer", again.Code, got)
	}
	if n := f.a.Count(`SELECT count(*) FROM payments WHERE order_id = $1`, x.o.ID); n != 1 {
		t.Errorf("payments after a replay = %d, want 1", n)
	}
	// A second key is a second attempt: a new payment id, a new hash.
	other := apptest.Decode[payhere.Checkout](t, f.start(f.me, x.o.ID, x.o.Version, "pay-key-0000000002"))
	if other.Fields["order_id"] == c.Fields["order_id"] || other.Fields["hash"] == c.Fields["hash"] {
		t.Error("a new attempt reused the payment id")
	}
}

func TestPay_Start_Refusals(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	stranger := f.a.As(f.a.SignedIn(fakes.Claims{Sub: "kamal", Email: "kamal@example.com", Name: "Kamal"}))
	path := "/orders/" + x.o.ID.String() + "/payment"
	for name, tc := range map[string]struct {
		c       apptest.Client
		version int
		key     string
		status  int
		code    apperr.Code
	}{
		"no key":          {f.me, x.o.Version, "", 400, apperr.ValidationError},
		"stale version":   {f.me, x.o.Version - 1, "k-stale-0000000001", 409, apperr.StaleOrder},
		"another's order": {stranger, x.o.Version, "k-other-0000000001", 404, apperr.NotFound},
		"version zero":    {f.me, 0, "k-zero-00000000001", 400, apperr.ValidationError},
	} {
		w := tc.c.CallWith(http.MethodPost, path, `{"version":`+strconv.Itoa(tc.version)+`}`, map[string]string{"Idempotency-Key": tc.key})
		if e := apptest.ErrorOf(t, w); w.Code != tc.status || string(apperr.Code(e.Error.Code)) != string(tc.code) {
			t.Errorf("%s: %d %s, want %d %s", name, w.Code, w.Body, tc.status, tc.code)
		}
	}
	// Same key, another body: refused, not replayed.
	f.attempt(x)
	f.a.Exec(`DELETE FROM rate_counters`)
	if w := f.start(f.me, x.o.ID, x.o.Version+1, payKey); w.Code != http.StatusConflict {
		t.Errorf("same key, other body = %d %s", w.Code, w.Body)
	}
	// An order that is not waiting for payment cannot be paid.
	f.me.OK(http.MethodPost, "/orders/"+x.o.ID.String()+"/cancel", version(x.o.Version, ""))
	if w := f.start(f.me, x.o.ID, x.o.Version+1, "k-closed-0000000001"); w.Code != http.StatusConflict || apptest.ErrorOf(t, w).Error.Code != "INVALID_TRANSITION" {
		t.Errorf("paying a cancelled order = %d %s", w.Code, w.Body)
	}
}

func TestPay_Start_RateLimited(t *testing.T) {
	f := newFixture(t)
	x := f.unpaid()
	var last int
	for i := 0; i < 22; i++ {
		last = f.start(f.me, x.o.ID, x.o.Version, "rate-key-"+strconv.Itoa(1000000+i)).Code
	}
	if last != http.StatusTooManyRequests {
		t.Errorf("22nd attempt in an hour = %d, want 429", last)
	}
}

// A manual shop shows its bank and the order code as the reference; without a
// bank there is nothing to pay into.
func TestPay_Start_Manual(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	f.a.Exec(`UPDATE orders SET status = 'AWAITING_PAYMENT', version = 2 WHERE id = $1`, x.o.ID)
	f.unavailable(x, "k-nobank-0000000001") // the account has no bank yet

	var acc string
	if err := f.a.Env.Owner.QueryRow(t.Context(), `SELECT id::text FROM payment_accounts WHERE provider = 'MANUAL'`).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	bank := `{"bank":{"bankName":"Commercial Bank","accountName":"Krishna Super","accountNumber":"8001234567","branch":"Galle Road"}}`
	f.admin().OK(http.MethodPatch, "/admin/payment-accounts/"+acc, bank)

	w := f.start(f.me, x.o.ID, 2, payKey)
	if w.Code != http.StatusOK {
		t.Fatalf("start = %d %s", w.Code, w.Body)
	}
	c := apptest.Decode[manual.Checkout](t, w)
	if c.Provider != "MANUAL" || c.Reference != x.o.Code || c.Bank.AccountNumber != "8001234567" || c.AmountCents <= 0 {
		t.Errorf("checkout = %+v", c)
	}
	// The shop moved to PayHere after this order was placed: the order cannot take that route.
	f.usePayHere()
	f.unavailable(x, "k-switched-000000001")
}

func (f fixture) unavailable(x trio, key string) {
	f.a.T.Helper()
	if w := f.start(f.me, x.o.ID, 2, key); w.Code != http.StatusConflict || apptest.ErrorOf(f.a.T, w).Error.Code != "PAYMENT_NOT_AVAILABLE" {
		f.a.T.Errorf("start = %d %s, want PAYMENT_NOT_AVAILABLE", w.Code, w.Body)
	}
}
