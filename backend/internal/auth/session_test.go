package auth_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"gp/internal/auth"
	"gp/internal/shared/clock"
	"gp/internal/shared/logx"
	"gp/test/fakes"
)

// refreshURL is the route the browser calls with the refresh cookie.
const refreshURL = "/v1/auth/refresh"

func TestMeAndBootstrap(t *testing.T) {
	a := newApp(t)
	in := a.signIn(fakes.Claims{Sub: "g1", Name: "Nimal"})
	at := cookies(in)[auth.AccessCookie]

	w := a.call(http.MethodGet, "/v1/me", "", at)
	if w.Code != http.StatusOK || decode[auth.Me](t, w).Name != "Nimal" {
		t.Fatalf("GET /me = %d %s", w.Code, w.Body)
	}
	boot := decode[struct{ Me *auth.Me }](t, a.call(http.MethodGet, "/v1/bootstrap", "", at))
	if boot.Me == nil || boot.Me.Name != "Nimal" {
		t.Fatalf("bootstrap me = %+v", boot.Me)
	}

	// Signed out: /me is 401 and /bootstrap still works with me = null.
	if w := a.call(http.MethodGet, "/v1/me", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /me signed out = %d", w.Code)
	}
	if boot := decode[struct{ Me *auth.Me }](t, a.call(http.MethodGet, "/v1/bootstrap", "")); boot.Me != nil {
		t.Fatalf("bootstrap me signed out = %+v", boot.Me)
	}
	// A forged cookie is the same as no cookie.
	if w := a.call(http.MethodGet, "/v1/me", "", &http.Cookie{Name: auth.AccessCookie, Value: "forged"}); w.Code != http.StatusUnauthorized {
		t.Fatalf("GET /me forged = %d", w.Code)
	}
}

func TestRefreshRotates(t *testing.T) {
	a := newApp(t)
	rt := cookies(a.signIn(fakes.Claims{}))[auth.RefreshCookie]

	w := a.call(http.MethodPost, refreshURL, "", rt)
	if w.Code != http.StatusNoContent {
		t.Fatalf("refresh = %d %s", w.Code, w.Body)
	}
	got := cookies(w)
	if got[auth.RefreshCookie] == nil || got[auth.RefreshCookie].Value == rt.Value || got[auth.AccessCookie] == nil {
		t.Fatalf("cookies = %v", got)
	}
	// The new access token works.
	if w := a.call(http.MethodGet, "/v1/me", "", got[auth.AccessCookie]); w.Code != http.StatusOK {
		t.Fatalf("GET /me with refreshed token = %d", w.Code)
	}
	// Both tokens belong to one family.
	if n := a.count(`SELECT count(DISTINCT family_id) FROM sessions`); n != 1 {
		t.Fatalf("families = %d", n)
	}
}

func TestRefreshWithoutOrWithBadCookie(t *testing.T) {
	a := newApp(t)
	for name, cs := range map[string][]*http.Cookie{
		"no cookie": nil,
		"unknown":   {{Name: auth.RefreshCookie, Value: "never-issued"}},
	} {
		w := a.call(http.MethodPost, refreshURL, "", cs...)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d", name, w.Code)
		}
		if c := cookies(w)[auth.RefreshCookie]; c == nil || c.MaxAge >= 0 {
			t.Errorf("%s: the dead cookie was not cleared", name)
		}
	}
}

// AUTHN-03: using a token that was already swapped revokes the whole family
// and raises the alarm event.
func TestRefreshReuseRevokesFamily(t *testing.T) {
	var logs bytes.Buffer
	logx.SetDefault(logx.New(slog.LevelInfo, &logs))
	t.Cleanup(func() { logx.SetDefault(logx.New(slog.LevelInfo, os.Stdout)) })

	a := newApp(t)
	old := cookies(a.signIn(fakes.Claims{}))[auth.RefreshCookie]
	fresh := cookies(a.call(http.MethodPost, refreshURL, "", old))[auth.RefreshCookie]

	if w := a.call(http.MethodPost, refreshURL, "", old); w.Code != http.StatusUnauthorized {
		t.Fatalf("reuse = %d, want 401", w.Code)
	}
	if !bytes.Contains(logs.Bytes(), []byte(`"msg":"auth.refresh_reuse"`)) {
		t.Fatalf("no auth.refresh_reuse event in logs: %s", logs.String())
	}
	// The thief and the real user are both signed out.
	if w := a.call(http.MethodPost, refreshURL, "", fresh); w.Code != http.StatusUnauthorized {
		t.Fatalf("newest token after reuse = %d, want 401", w.Code)
	}
	if n := a.count(`SELECT count(*) FROM sessions WHERE revoked_at IS NULL`); n != 0 {
		t.Fatalf("%d sessions still active", n)
	}
}

func TestRefreshExpired(t *testing.T) {
	a := newApp(t)
	rt := cookies(a.signIn(fakes.Claims{}))[auth.RefreshCookie]
	defer clock.Freeze(time.Now().Add(auth.RefreshTTL + time.Hour))()
	if w := a.call(http.MethodPost, refreshURL, "", rt); w.Code != http.StatusUnauthorized {
		t.Fatalf("expired refresh = %d", w.Code)
	}
}

// AUTHN-08: blocking a user ends their access at the next refresh and sign-in.
func TestBlockedUserCannotRefreshOrSignIn(t *testing.T) {
	a := newApp(t)
	rt := cookies(a.signIn(fakes.Claims{Sub: "g1"}))[auth.RefreshCookie]
	if _, err := a.env.Owner.Exec(context.Background(), `UPDATE users SET status = 'BLOCKED'`); err != nil {
		t.Fatal(err)
	}
	if w := a.call(http.MethodPost, refreshURL, "", rt); w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh = %d, want 401", w.Code)
	}
	if n := a.count(`SELECT count(*) FROM sessions WHERE revoked_at IS NULL`); n != 0 {
		t.Fatalf("%d sessions still active", n)
	}
	if w := a.signIn(fakes.Claims{Sub: "g1"}); w.Code != http.StatusForbidden {
		t.Fatalf("sign-in = %d, want 403", w.Code)
	}
}

func TestLogout(t *testing.T) {
	a := newApp(t)
	first := a.signIn(fakes.Claims{Sub: "g1"})
	second := a.signIn(fakes.Claims{Sub: "g1"}) // another device
	at, rt := cookies(first)[auth.AccessCookie], cookies(first)[auth.RefreshCookie]

	w := a.call(http.MethodPost, "/v1/auth/logout", "", at)
	if w.Code != http.StatusNoContent {
		t.Fatalf("logout = %d %s", w.Code, w.Body)
	}
	for _, name := range []string{auth.AccessCookie, auth.RefreshCookie} {
		if c := cookies(w)[name]; c == nil || c.MaxAge >= 0 || c.Value != "" {
			t.Errorf("%s was not cleared: %+v", name, c)
		}
	}
	if w := a.call(http.MethodPost, refreshURL, "", rt); w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout = %d", w.Code)
	}
	// The other device is still signed in.
	if w := a.call(http.MethodPost, refreshURL, "", cookies(second)[auth.RefreshCookie]); w.Code != http.StatusNoContent {
		t.Fatalf("other device refresh = %d", w.Code)
	}

	// "all" signs out every device.
	third := a.signIn(fakes.Claims{Sub: "g1"})
	if w := a.call(http.MethodPost, "/v1/auth/logout", `{"all":true}`, cookies(third)[auth.AccessCookie]); w.Code != http.StatusNoContent {
		t.Fatalf("logout all = %d", w.Code)
	}
	if n := a.count(`SELECT count(*) FROM sessions WHERE revoked_at IS NULL`); n != 0 {
		t.Fatalf("%d sessions still active", n)
	}
}

func TestLogoutNeedsSignIn(t *testing.T) {
	a := newApp(t)
	if w := a.call(http.MethodPost, "/v1/auth/logout", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("logout signed out = %d", w.Code)
	}
}

// A user cannot end another user's sessions, even with a session id they know.
func TestLogoutOnlyTouchesOwnSessions(t *testing.T) {
	a := newApp(t)
	a.signIn(fakes.Claims{Sub: "victim", Email: "victim@example.test"})
	mine := a.signIn(fakes.Claims{Sub: "me", Email: "me@example.test"})
	a.call(http.MethodPost, "/v1/auth/logout", `{"all":true}`, cookies(mine)[auth.AccessCookie])
	if n := a.count(`SELECT count(*) FROM sessions s JOIN users u ON u.id = s.user_id WHERE u.google_sub = 'victim' AND s.revoked_at IS NULL`); n != 1 {
		t.Fatalf("victim's active sessions = %d, want 1", n)
	}
}
