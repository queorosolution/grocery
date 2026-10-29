package turnstile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"gp/internal/shared/apperr"
	"gp/internal/shared/secrets"
)

type fakeSecrets map[string]string

func (f fakeSecrets) Get(_ context.Context, name string) (string, error) {
	if v, ok := f[name]; ok {
		return v, nil
	}
	return "", secrets.ErrNotFound
}

func TestVerify(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r.PostForm
		switch r.PostForm.Get("response") {
		case "good":
			_, _ = w.Write([]byte(`{"success":true}`))
		case "bad":
			_, _ = w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
		case "down":
			http.Error(w, "boom", http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(`not json`))
		}
	}))
	defer srv.Close()
	c := newClient(fakeSecrets{secretKey: "s3cret"}, srv.URL)
	ctx := context.Background()

	if err := c.Verify(ctx, "good", "203.0.113.9"); err != nil {
		t.Fatalf("good token: %v", err)
	}
	if got.Get("secret") != "s3cret" || got.Get("remoteip") != "203.0.113.9" {
		t.Errorf("form sent: %v", got)
	}
	cases := map[string]apperr.Code{
		"bad":                               apperr.BotCheckFailed,
		"":                                  apperr.BotCheckFailed,
		"   ":                               apperr.BotCheckFailed,
		string(make([]byte, maxTokenLen+1)): apperr.BotCheckFailed,
		"down":                              apperr.Internal,
		"garbage":                           apperr.Internal,
	}
	for token, want := range cases {
		err := c.Verify(ctx, token, "")
		if !apperr.Is(err, want) {
			t.Errorf("token %.8q: %v, want %s", token, err, want)
		}
	}
	// No secret configured: fail, never pass.
	if err := newClient(fakeSecrets{}, srv.URL).Verify(ctx, "good", ""); err == nil {
		t.Error("a missing secret must not pass the check")
	}
	// Cloudflare unreachable: an error, not a pass.
	srv.Close()
	if err := c.Verify(ctx, "good", ""); !apperr.Is(err, apperr.Internal) {
		t.Errorf("unreachable: %v, want INTERNAL", err)
	}
}
