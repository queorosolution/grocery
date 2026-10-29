package order_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"

	"github.com/google/uuid"

	"gp/internal/payment/payhere"
	"gp/internal/shared/money"
	"gp/internal/shop"
	"gp/test/apptest"
)

const (
	merchantID     = "1230001"
	merchantSecret = "Mrch-Secret-ABC123xyz"
	payhereRef     = "320032315782" // PayHere's own id of the payment
	payKey         = "pay-key-0000000001"
)

var (
	adminsMu sync.Mutex
	admins   = map[*apptest.App]apptest.Client{}
)

// admin is the super admin, signed in once per test app: a second authenticator
// set-up for the same account is refused.
func (f fixture) admin() apptest.Client {
	adminsMu.Lock()
	defer adminsMu.Unlock()
	if c, ok := admins[f.a]; ok {
		return c
	}
	c := f.a.AsAdmin()
	admins[f.a] = c
	return c
}

// usePayHere gives the platform a PayHere account and points the shop at it.
func (f fixture) usePayHere() uuid.UUID {
	f.a.T.Helper()
	body := `{"ownerType":"PLATFORM","provider":"PAYHERE","mode":"SANDBOX","merchantId":"` + merchantID + `","merchantSecret":"` + merchantSecret + `"}`
	acc := apptest.Decode[shop.Account](f.a.T, f.admin().OK(http.MethodPost, "/admin/payment-accounts", body))
	f.a.Exec(`UPDATE shop_settings SET payment_provider = 'PAYHERE', payment_account_id = $2 WHERE shop_id = $1`, f.shop, acc.ID)
	return acc.ID
}

// unpaid is an approved order of a PayHere shop: the customer can pay.
func (f fixture) unpaid() trio {
	f.a.T.Helper()
	f.usePayHere()
	x := f.awaiting()
	x.o = f.get(f.me, x.o.ID)
	return x
}

// start sends POST /orders/{id}/payment.
func (f fixture) start(c apptest.Client, id uuid.UUID, v int, key string) *httptest.ResponseRecorder {
	return c.CallWith(http.MethodPost, "/orders/"+id.String()+"/payment", fmt.Sprintf(`{"version":%d}`, v), map[string]string{"Idempotency-Key": key})
}

// attempt starts a payment and returns the checkout the browser would post.
func (f fixture) attempt(x trio) payhere.Checkout {
	f.a.T.Helper()
	w := f.start(f.me, x.o.ID, x.o.Version, payKey)
	if w.Code != http.StatusOK {
		f.a.T.Fatalf("start = %d %s", w.Code, w.Body)
	}
	return apptest.Decode[payhere.Checkout](f.a.T, w)
}

// callback is what PayHere posts for the checkout, signed with the merchant secret.
// extra pairs are added after signing, as PayHere's card fields are.
func callback(c payhere.Checkout, status string, extra ...string) url.Values {
	return signed(c.Fields["order_id"], c.Fields["amount"], status, extra...)
}

// signSig is the md5sig of a callback with the merchant's secret.
func signSig(merchant, paymentID, amount, currency, status string) string {
	return payhere.NotifySignature(merchant, paymentID, amount, currency, status, merchantSecret)
}

func signed(paymentID, amount, status string, extra ...string) url.Values {
	v := url.Values{
		"merchant_id": {merchantID}, "order_id": {paymentID}, "payment_id": {payhereRef}, "payhere_amount": {amount},
		"payhere_currency": {"LKR"}, "status_code": {status}, "method": {"VISA"}, "status_message": {"Successfully received the payment"},
	}
	v.Set("md5sig", payhere.NotifySignature(merchantID, paymentID, amount, "LKR", status, merchantSecret))
	for i := 0; i+1 < len(extra); i += 2 {
		v.Set(extra[i], extra[i+1])
	}
	return v
}

func (f fixture) notify(v url.Values) *httptest.ResponseRecorder {
	return f.a.PostForm("/v1/webhooks/payhere", v)
}

// paid starts a payment and delivers the success callback.
func (f fixture) paid(x trio) payhere.Checkout {
	f.a.T.Helper()
	c := f.attempt(x)
	if w := f.notify(callback(c, "2")); w.Code != http.StatusOK {
		f.a.T.Fatalf("notify = %d %s", w.Code, w.Body)
	}
	return c
}

func (f fixture) payments(id uuid.UUID, status string) int {
	return f.a.Count(`SELECT count(*) FROM payments WHERE order_id = $1 AND status = $2`, id, status)
}

func (f fixture) ledger(id uuid.UUID) int {
	return f.a.Count(`SELECT count(*) FROM ledger_entries WHERE order_id = $1`, id)
}

func (f fixture) events(typ string, id uuid.UUID) int {
	return f.a.Count(`SELECT count(*) FROM outbox WHERE type = $1 AND entity_id = $2`, typ, id.String())
}

// totalOf is the order's total as the gateway form writes it.
func (f fixture) totalOf(id uuid.UUID) string {
	return money.Decimal(money.Cents(f.a.Count(`SELECT total_cents FROM orders WHERE id = $1`, id)))
}
