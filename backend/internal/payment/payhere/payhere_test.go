package payhere

import (
	"context"
	"crypto/md5" // #nosec G501 -- the test recomputes PayHere's MD5 scheme independently
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/payment"
	"gp/internal/shop"
)

type secret string

func (s secret) Get(context.Context, string) (string, error) { return string(s), nil }

type noSecret struct{}

func (noSecret) Get(context.Context, string) (string, error) { return "", errors.New("gone") }

// md5Upper is written out here on purpose, so a mistake in upperMD5 shows.
func md5Upper(s string) string { return fmt.Sprintf("%X", md5.Sum([]byte(s))) } // #nosec G401

func TestCheckoutHash(t *testing.T) {
	// UPPER(MD5(merchant_id + order_id + amount + currency + UPPER(MD5(secret))))
	want := md5Upper("1230001" + "7f0c9a52" + "1585.00" + "LKR" + md5Upper("secret-1"))
	if got := CheckoutHash("1230001", "7f0c9a52", "1585.00", "LKR", "secret-1"); got != want || got != strings.ToUpper(got) || len(got) != 32 {
		t.Errorf("hash = %q, want %q", got, want)
	}
	if CheckoutHash("1230001", "7f0c9a52", "1585.01", "LKR", "secret-1") == want {
		t.Error("the amount is not in the hash")
	}
}

func TestNotifySignature(t *testing.T) {
	want := md5Upper("1230001" + "pay" + "10.00" + "LKR" + "2" + md5Upper("secret-1"))
	if got := NotifySignature("1230001", "pay", "10.00", "LKR", "2", "secret-1"); got != want {
		t.Errorf("md5sig = %q, want %q", got, want)
	}
	if NotifySignature("1230001", "pay", "10.00", "LKR", "-2", "secret-1") == want {
		t.Error("the status code is not in the signature")
	}
}

func TestParseNotify(t *testing.T) {
	acc := shop.PayAccount{ID: uuid.New(), MerchantID: "1230001", SecretRef: "payhere/x"}
	pay := payment.Payment{ID: uuid.New(), AmountCents: 158500}
	p := New(secret("secret-1"), "https://www.example.test", "https://api.example.test/v1/webhooks/payhere")
	form := func(edit func(url.Values)) url.Values {
		v := url.Values{"merchant_id": {"1230001"}, "order_id": {pay.ID.String()}, "payment_id": {"320032315782"}, "payhere_amount": {"1585.00"},
			"payhere_currency": {"LKR"}, "status_code": {"2"}, "method": {"VISA"}, "card_no": {"************1234"}, "card_holder_name": {"N P"}}
		if edit != nil {
			edit(v)
		}
		v.Set("md5sig", NotifySignature(v.Get("merchant_id"), v.Get("order_id"), v.Get("payhere_amount"), v.Get("payhere_currency"), v.Get("status_code"), "secret-1"))
		return v
	}
	got, err := p.ParseNotify(t.Context(), acc, pay, form(nil))
	if err != nil || got.Ref != "320032315782" || got.Code != 2 || got.Method != "VISA" {
		t.Fatalf("good callback: %+v, %v", got, err)
	}
	for _, k := range []string{"card_no", "card_holder_name", "md5sig"} {
		if _, kept := got.Raw[k]; kept {
			t.Errorf("raw keeps %s", k)
		}
	}
	// Lower case in the signature is the same signature.
	lower := form(nil)
	lower.Set("md5sig", strings.ToLower(lower.Get("md5sig")))
	if _, err := p.ParseNotify(t.Context(), acc, pay, lower); err != nil {
		t.Errorf("lower case md5sig: %v", err)
	}
	for name, tc := range map[string]struct {
		in   url.Values
		want error
	}{
		"wrong sig":    {func() url.Values { v := form(nil); v.Set("md5sig", strings.Repeat("0", 32)); return v }(), payment.ErrBadSignature},
		"empty sig":    {func() url.Values { v := form(nil); v.Del("md5sig"); return v }(), payment.ErrBadSignature},
		"amount moved": {func() url.Values { v := form(nil); v.Set("payhere_amount", "1.00"); return v }(), payment.ErrBadSignature},
		"merchant":     {form(func(v url.Values) { v.Set("merchant_id", "999") }), payment.ErrMismatch},
		"currency":     {form(func(v url.Values) { v.Set("payhere_currency", "USD") }), payment.ErrMismatch},
		"amount":       {form(func(v url.Values) { v.Set("payhere_amount", "1584.99") }), payment.ErrMismatch},
		"amount form":  {form(func(v url.Values) { v.Set("payhere_amount", "1585") }), payment.ErrMismatch},
		"status code":  {form(func(v url.Values) { v.Set("status_code", "two") }), payment.ErrMismatch},
	} {
		if _, err := p.ParseNotify(t.Context(), acc, pay, tc.in); !errors.Is(err, tc.want) {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
	if _, err := New(noSecret{}, "", "").ParseNotify(t.Context(), acc, pay, form(nil)); err == nil || errors.Is(err, payment.ErrBadSignature) {
		t.Errorf("a missing secret must be our error, not a bad signature: %v", err)
	}
}
