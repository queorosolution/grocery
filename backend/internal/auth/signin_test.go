package auth_test

import (
	"net/http"
	"testing"

	"gp/internal/auth"
	"gp/test/apptest"
	"gp/test/fakes"
)

func TestSignInCreatesUserAndCookies(t *testing.T) {
	a := apptest.New(t)
	w := a.SignIn(fakes.Claims{Sub: "g1", Email: "Nimal@Example.test", Name: "Nimal"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body)
	}

	me := apptest.Decode[auth.Me](t, w)
	if me.Email != "nimal@example.test" || me.Name != "Nimal" || !me.NeedsOnboarding || me.PhoneVerified ||
		len(me.Roles) != 0 || me.AdminTotp != auth.AdminTotpNotRequired {
		t.Fatalf("me = %+v", me)
	}

	// AUTHN-04: flags, paths, and no Domain (host-only).
	got := apptest.Cookies(w)
	at, rt := got[auth.AccessCookie], got[auth.RefreshCookie]
	if at == nil || rt == nil {
		t.Fatalf("cookies = %v", got)
	}
	for _, c := range []*http.Cookie{at, rt} {
		if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Domain != "" {
			t.Errorf("%s flags = %+v", c.Name, c)
		}
	}
	if at.Path != "/" || rt.Path != "/v1/auth" {
		t.Errorf("paths = %q, %q", at.Path, rt.Path)
	}

	// Only a hash of the refresh token is stored, with a hashed IP.
	if n := a.Count(`SELECT count(*) FROM sessions WHERE refresh_hash = $1`, []byte(rt.Value)); n != 0 {
		t.Fatal("the refresh token is stored in clear")
	}
	if n := a.Count(`SELECT count(*) FROM sessions WHERE ip_hash IS NOT NULL AND octet_length(ip_hash) = 32`); n != 1 {
		t.Fatalf("sessions with ip hash = %d", n)
	}
}

func TestSignInTwiceKeepsTheSameUser(t *testing.T) {
	a := apptest.New(t)
	first := apptest.Decode[auth.Me](t, a.SignIn(fakes.Claims{Sub: "g1", Name: "Nimal"}))
	second := apptest.Decode[auth.Me](t, a.SignIn(fakes.Claims{Sub: "g1", Name: "Changed In Google"}))
	if first.ID != second.ID || second.Name != "Nimal" {
		t.Fatalf("first %+v second %+v", first, second)
	}
	if n := a.Count(`SELECT count(*) FROM sessions`); n != 2 {
		t.Fatalf("sessions = %d, want 2", n)
	}
}

func TestSignInRefusals(t *testing.T) {
	no := false
	tests := map[string]fakes.Claims{
		"token for another app": {Audience: "other.apps.example.test"},
		"unverified e-mail":     {EmailVerified: &no},
		"unknown key":           {SignedByOtherKey: true},
	}
	for name, c := range tests {
		t.Run(name, func(t *testing.T) {
			a := apptest.New(t)
			w := a.SignIn(c)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", w.Code)
			}
			if len(w.Result().Cookies()) != 0 {
				t.Fatal("cookies were set")
			}
			if n := a.Count(`SELECT count(*) FROM users`); n != 0 {
				t.Fatal("a user was created")
			}
		})
	}
}

func TestSignInBodyIsChecked(t *testing.T) {
	a := apptest.New(t)
	for name, body := range map[string]string{
		"empty token":   `{"idToken":""}`,
		"unknown field": `{"idToken":"x","role":"SUPER_ADMIN"}`,
		"not json":      `nope`,
	} {
		if w := a.Call(http.MethodPost, "/v1/auth/google", body); w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, w.Code)
		}
	}
}

func TestSuperAdminEmailGetsMembership(t *testing.T) {
	a := apptest.New(t)
	boss := apptest.Decode[auth.Me](t, a.SignIn(fakes.Claims{Sub: "boss", Email: apptest.AdminEmail}))
	if len(boss.Roles) != 1 || boss.Roles[0].Role != "SUPER_ADMIN" || boss.Roles[0].ShopID != nil {
		t.Fatalf("roles = %+v", boss.Roles)
	}
	if boss.AdminTotp != auth.AdminTotpSetupNeeded {
		t.Fatalf("adminTotp = %s, want SETUP_NEEDED", boss.AdminTotp)
	}
	// Signing in again does not add a second membership.
	a.SignIn(fakes.Claims{Sub: "boss", Email: apptest.AdminEmail})
	if n := a.Count(`SELECT count(*) FROM memberships`); n != 1 {
		t.Fatalf("memberships = %d", n)
	}

	other := apptest.Decode[auth.Me](t, a.SignIn(fakes.Claims{Sub: "other", Email: "other@example.test"}))
	if len(other.Roles) != 0 {
		t.Fatalf("a normal user got roles: %+v", other.Roles)
	}
}

func TestSignInRateLimitPerIP(t *testing.T) {
	a := apptest.New(t)
	for i := 0; i < 10; i++ {
		w := a.CallFrom("203.0.113.5", http.MethodPost, "/v1/auth/google", `{"idToken":"junk"}`)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, w.Code)
		}
	}
	if w := a.CallFrom("203.0.113.5", http.MethodPost, "/v1/auth/google", `{"idToken":"junk"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("11th attempt: status = %d, want 429", w.Code)
	}
	// Another address is not affected.
	if w := a.SignInFrom("203.0.113.6", fakes.Claims{}); w.Code != http.StatusOK {
		t.Fatalf("other IP: status = %d", w.Code)
	}
}
