package sms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"gp/internal/notify"
)

type key string

func (k key) Get(context.Context, string) (string, error) { return string(k), nil }

var msg = notify.Message{To: "+94771234567", Template: notify.OrderDelivered, Vars: map[string]string{"app": "NearShop", "orderCode": "KS-1"}}

func TestSend_PostsTheTextWithTheKey(t *testing.T) {
	var got request
	var auth, ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ctype = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"id":"sms-77"}`))
	}))
	defer srv.Close()
	ref, cost, err := newClient(srv.URL, "NearShop", key("secret-key")).Send(context.Background(), msg)
	if err != nil || ref != "sms-77" || cost != 1 {
		t.Fatalf("send = %q, %d, %v", ref, cost, err)
	}
	if auth != "Bearer secret-key" || ctype != "application/json" || got.Sender != "NearShop" || got.To != "94771234567" ||
		got.Message != "NearShop: order KS-1 delivered. Thank you!" {
		t.Errorf("request = %+v, auth %q", got, auth)
	}
}

// The provider's answer may repeat the number or the text, so no error carries it.
func TestSend_ErrorsNeverCarryTheAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad number +94771234567, text: order KS-1", http.StatusBadRequest)
	}))
	defer srv.Close()
	_, _, err := newClient(srv.URL, "NearShop", key("k")).Send(context.Background(), msg)
	if err == nil || strings.Contains(err.Error(), "94771234567") || strings.Contains(err.Error(), "KS-1") || !strings.Contains(err.Error(), "400") {
		t.Errorf("err = %v", err)
	}
}

// A redirect is a failure, and the key is not sent on to the new address.
func TestSend_DoesNotFollowRedirects(t *testing.T) {
	var hits atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	if _, _, err := newClient(srv.URL, "NearShop", key("k")).Send(context.Background(), msg); err == nil {
		t.Error("a redirect must fail")
	}
	if hits.Load() != 0 {
		t.Error("the redirect was followed")
	}
}

func TestSend_NoKeyNoRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	c := newClient(srv.URL, "NearShop", failing{})
	if _, _, err := c.Send(context.Background(), msg); err == nil || hits.Load() != 0 {
		t.Errorf("err = %v, hits = %d", err, hits.Load())
	}
}

type failing struct{}

func (failing) Get(context.Context, string) (string, error) { return "", context.DeadlineExceeded }

func TestNew_NeedsHTTPS(t *testing.T) {
	for _, bad := range []string{"http://sms.example.lk/send", "ftp://x", "https://user:pw@sms.example.lk", "", "sms.example.lk"} {
		if _, err := New(bad, "S", key("k")); err == nil {
			t.Errorf("New(%q): want an error", bad)
		}
	}
	if _, err := New("https://sms.example.lk/send", "S", key("k")); err != nil {
		t.Error(err)
	}
}
