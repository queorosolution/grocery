package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gp/internal/payment/payhere"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestNotifyIsSignedForTheAPI(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got, _ = url.ParseQuery(string(b))
		if r.URL.Path != "/v1/webhooks/payhere" || r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("request = %s %s", r.URL.Path, r.Header.Get("Content-Type"))
		}
	}))
	defer srv.Close()
	e := env(map[string]string{"PAYHERE_MERCHANT_ID": "1230001", "PAYHERE_MERCHANT_SECRET": "s3cret-value"})
	err := run(t.Context(), []string{"-base", srv.URL, "-payment", "pay-1", "-amount", "10.00", "-status", "-2", "-card", "-times", "2"}, e, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if want := payhere.NotifySignature("1230001", "pay-1", "10.00", "LKR", "-2", "s3cret-value"); got.Get("md5sig") != want || got.Get("status_code") != "-2" {
		t.Errorf("form = %v", got)
	}
	if got.Get("card_no") == "" {
		t.Error("-card added no card fields")
	}
	if strings.Contains(got.Encode(), "s3cret-value") {
		t.Error("the secret is in the form")
	}
}

func TestOnlyLoopback(t *testing.T) {
	for _, ok := range []string{"http://localhost:8080", "http://127.0.0.1:9", "http://[::1]:8080"} {
		if _, err := notifyURL(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"https://api.example.lk", "http://10.0.0.5:8080", "http://localhost.evil.test", "localhost", ""} {
		if _, err := notifyURL(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestNeedsItsInputs(t *testing.T) {
	e := env(map[string]string{"PAYHERE_MERCHANT_ID": "1", "PAYHERE_MERCHANT_SECRET": "x"})
	for _, args := range [][]string{{}, {"-payment", "p"}, {"-payment", "p", "-amount", "1.00", "-times", "0"}} {
		if err := run(t.Context(), args, e, http.DefaultClient); err == nil {
			t.Errorf("%v was accepted", args)
		}
	}
	if err := run(t.Context(), []string{"-payment", "p", "-amount", "1.00"}, env(nil), http.DefaultClient); err == nil {
		t.Error("ran without the merchant secret")
	}
}
