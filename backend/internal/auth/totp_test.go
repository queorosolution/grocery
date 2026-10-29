package auth_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"

	"gp/internal/auth"
	"gp/internal/shared/clock"
	"gp/test/apptest"
	"gp/test/fakes"
)

type setupBody struct {
	OtpauthURI   string `json:"otpauthUri"`
	SecretMasked string `json:"secretMasked"`
}

type errBody struct{ Error struct{ Code string } }

func codeAt(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	c, err := totp.GenerateCodeCustom(secret, at, totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func verify(a *apptest.App, s apptest.Session, code string) *httptest.ResponseRecorder {
	return a.Call(http.MethodPost, "/v1/auth/totp/verify", `{"code":"`+code+`"}`, s.Cookies()...)
}

// setup signs in as the super admin and starts authenticator setup.
func setup(t *testing.T, a *apptest.App) (apptest.Session, string) {
	t.Helper()
	s := a.SignedIn(fakes.Claims{Sub: "boss", Email: apptest.AdminEmail})
	w := a.Call(http.MethodPost, "/v1/auth/totp/setup", "", s.Cookies()...)
	if w.Code != http.StatusOK {
		t.Fatalf("setup = %d %s", w.Code, w.Body)
	}
	body := apptest.Decode[setupBody](t, w)
	u, err := url.Parse(body.OtpauthURI)
	if err != nil || u.Scheme != "otpauth" || u.Host != "totp" || u.Query().Get("issuer") != "TestShop" {
		t.Fatalf("otpauth URI = %q", body.OtpauthURI)
	}
	secret := u.Query().Get("secret")
	if secret == "" || !strings.HasSuffix(body.SecretMasked, "••••••••") || strings.Contains(body.SecretMasked, secret) {
		t.Fatalf("masked secret = %q", body.SecretMasked)
	}
	return s, secret
}

// AUTHN-05: an admin route needs a session that passed the authenticator check.
func TestAdminRouteNeedsTotp(t *testing.T) {
	defer clock.Freeze(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC))()
	a := apptest.New(t)
	s, secret := setup(t, a)

	// Signed in as admin but no code yet.
	w := a.Call(http.MethodGet, "/v1/admin/users", "", s.Cookies()...)
	if w.Code != http.StatusUnauthorized || apptest.Decode[errBody](t, w).Error.Code != "TOTP_REQUIRED" {
		t.Fatalf("admin route without code = %d %s", w.Code, w.Body)
	}

	w = verify(a, s, codeAt(t, secret, clock.Now()))
	if w.Code != http.StatusOK {
		t.Fatalf("verify = %d %s", w.Code, w.Body)
	}
	if me := apptest.Decode[auth.Me](t, w); me.AdminTotp != auth.AdminTotpOK {
		t.Fatalf("adminTotp = %s", me.AdminTotp)
	}
	at := apptest.Cookies(w)[auth.AccessCookie]
	if at == nil || apptest.Cookies(w)[auth.RefreshCookie] != nil {
		t.Fatal("verify must replace only the access cookie")
	}
	if w := a.Call(http.MethodGet, "/v1/admin/users", "", at, s.Refresh); w.Code != http.StatusOK {
		t.Fatalf("admin route with code = %d %s", w.Code, w.Body)
	}

	// The check survives a refresh, but a new sign-in starts again at CODE_NEEDED.
	r := a.Call(http.MethodPost, "/v1/auth/refresh", "", s.Refresh)
	if w := a.Call(http.MethodGet, "/v1/admin/users", "", apptest.Cookies(r)[auth.AccessCookie]); w.Code != http.StatusOK {
		t.Fatalf("admin route after refresh = %d", w.Code)
	}
	other := a.SignedIn(fakes.Claims{Sub: "boss", Email: apptest.AdminEmail})
	if other.Me.AdminTotp != auth.AdminTotpCodeNeeded {
		t.Fatalf("second device adminTotp = %s, want CODE_NEEDED", other.Me.AdminTotp)
	}
	if w := a.Call(http.MethodGet, "/v1/admin/users", "", other.Cookies()...); w.Code != http.StatusUnauthorized {
		t.Fatalf("second device admin route = %d", w.Code)
	}
}

func TestTotpSecretIsEncryptedAtRest(t *testing.T) {
	a := apptest.New(t)
	_, secret := setup(t, a)
	var stored []byte
	if err := a.Env.Owner.QueryRow(t.Context(), `SELECT secret_enc FROM admin_totp`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) == 0 || bytes.Contains(stored, []byte(secret)) {
		t.Fatal("the authenticator secret is stored in clear")
	}
}

// AUTHN-06: a code works once, and five tries are all a user gets in 15 minutes.
func TestTotpReplayAndBruteForce(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	defer clock.Freeze(now)()
	a := apptest.New(t)
	s, secret := setup(t, a)

	code := codeAt(t, secret, now)
	if w := verify(a, s, code); w.Code != http.StatusOK {
		t.Fatalf("first use = %d %s", w.Code, w.Body)
	}
	w := verify(a, s, code)
	if w.Code != http.StatusUnauthorized || apptest.Decode[errBody](t, w).Error.Code != "TOTP_INVALID" {
		t.Fatalf("replay = %d %s", w.Code, w.Body)
	}
	// An older step is refused too; the next step works.
	if w := verify(a, s, codeAt(t, secret, now.Add(-30*time.Second))); w.Code != http.StatusUnauthorized {
		t.Fatalf("older step = %d", w.Code)
	}
	clock.Freeze(now.Add(30 * time.Second))
	// That was 3 tries so far; the next one is the 4th.
	if w := verify(a, s, codeAt(t, secret, clock.Now())); w.Code != http.StatusOK {
		t.Fatalf("next step = %d %s", w.Code, w.Body)
	}
	if w := verify(a, s, "000000"); w.Code != http.StatusUnauthorized {
		t.Fatalf("5th try = %d, want 401", w.Code)
	}
	// The 6th try is refused even with a good code.
	clock.Freeze(now.Add(60 * time.Second))
	if w := verify(a, s, codeAt(t, secret, clock.Now())); w.Code != http.StatusTooManyRequests {
		t.Fatalf("6th try = %d, want 429", w.Code)
	}
}

func TestTotpGuards(t *testing.T) {
	a := apptest.New(t)
	user := a.SignedIn(fakes.Claims{Sub: "u1", Email: "u1@example.test"})
	boss := a.SignedIn(fakes.Claims{Sub: "boss", Email: apptest.AdminEmail})

	for name, call := range map[string]func() int{
		"setup signed out":         func() int { return a.Call(http.MethodPost, "/v1/auth/totp/setup", "").Code },
		"setup as a customer":      func() int { return a.Call(http.MethodPost, "/v1/auth/totp/setup", "", user.Cookies()...).Code },
		"verify as a customer":     func() int { return verify(a, user, "123456").Code },
		"verify before setup":      func() int { return verify(a, boss, "123456").Code },
		"verify with a bad format": func() int { return verify(a, boss, "12ab56").Code },
		"verify with unknown field": func() int {
			return a.Call(http.MethodPost, "/v1/auth/totp/verify", `{"code":"123456","x":1}`, boss.Cookies()...).Code
		},
	} {
		got := call()
		want := map[string]int{
			"setup signed out": 401, "setup as a customer": 403, "verify as a customer": 403,
			"verify before setup": 403, "verify with a bad format": 400, "verify with unknown field": 400,
		}[name]
		if got != want {
			t.Errorf("%s = %d, want %d", name, got, want)
		}
	}
}

// Setup can be repeated until the first code is accepted; after that the
// secret is fixed and nobody can swap it.
func TestTotpSetupOnlyOnce(t *testing.T) {
	defer clock.Freeze(time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC))()
	a := apptest.New(t)
	s, first := setup(t, a)

	w := a.Call(http.MethodPost, "/v1/auth/totp/setup", "", s.Cookies()...)
	if w.Code != http.StatusOK {
		t.Fatalf("second setup before verify = %d", w.Code)
	}
	u, _ := url.Parse(apptest.Decode[setupBody](t, w).OtpauthURI)
	second := u.Query().Get("secret")
	if second == first {
		t.Fatal("the pending secret was not replaced")
	}
	if w := verify(a, s, codeAt(t, first, clock.Now())); w.Code != http.StatusUnauthorized {
		t.Fatalf("code from the replaced secret = %d, want 401", w.Code)
	}
	if w := verify(a, s, codeAt(t, second, clock.Now())); w.Code != http.StatusOK {
		t.Fatalf("code from the new secret = %d", w.Code)
	}
	if w := a.Call(http.MethodPost, "/v1/auth/totp/setup", "", s.Cookies()...); w.Code != http.StatusForbidden {
		t.Fatalf("setup after enabling = %d, want 403", w.Code)
	}
}
