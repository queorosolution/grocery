package http

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	nethttp "net/http"

	"github.com/google/uuid"

	"gp/internal/shared/authn"
	"gp/internal/shared/authz"
	"gp/internal/shared/config"
	"gp/internal/shared/enum"
)

const webOrigin = "https://app.example.test"

func testDeps() Deps {
	return Deps{Config: &config.Config{
		PlatformName: "TestShop", PlatformDomain: "example.test", SupportPhone: "+94770000000",
		AllowedOrigins: []string{webOrigin}, FeatureManualPay: true,
	}}
}

func do(h nethttp.Handler, method, path string, hdr map[string]string, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestEveryRouteHasValidGuard(t *testing.T) {
	seen := map[string]bool{}
	for _, rt := range Routes(testDeps()) {
		key := rt.Method + " " + rt.Pattern
		if seen[key] {
			t.Errorf("%s is registered twice", key)
		}
		seen[key] = true
		if rt.Handler == nil {
			t.Errorf("%s has no handler", key)
		}
		if err := rt.Guard.Validate(); err != nil {
			t.Errorf("%s: %v", key, err)
		}
	}
}

func TestGuardValidate(t *testing.T) {
	cases := map[Guard]bool{
		Public: true, User: true, Admin: true, Sig: true,
		Perm(enum.PermissionOrderReview): true,
		"":                               false, "perm:": false, "perm:nope": false, "PUBLIC": false, "anyone": false,
	}
	for g, ok := range cases {
		if got := g.Validate() == nil; got != ok {
			t.Errorf("Guard(%q) valid = %v, want %v", g, got, ok)
		}
	}
}

func TestHealth(t *testing.T) {
	w := do(New(testDeps()), "GET", "/v1/health", nil, "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"ok":true}` {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Request-Id") == "" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("missing standard headers: %v", w.Header())
	}
}

func TestBootstrapSignedOut(t *testing.T) {
	w := do(New(testDeps()), "GET", "/v1/bootstrap", nil, "")
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || w.Code != 200 {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
	p := got["platform"].(map[string]any)
	if p["name"] != "TestShop" || p["singleShopSlug"] != nil {
		t.Errorf("platform = %v", p)
	}
	if got["me"] != nil {
		t.Errorf("me = %v, want null", got["me"])
	}
	if got["features"].(map[string]any)["manualPayment"] != true {
		t.Errorf("features = %v", got["features"])
	}
}

func TestUnknownRouteIsJSON404(t *testing.T) {
	w := do(New(testDeps()), "GET", "/v1/nope", nil, "")
	if w.Code != 404 || !strings.Contains(w.Body.String(), `"NOT_FOUND"`) {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

// guardedRoutes is a throwaway table that proves each guard through the full chain.
func guardedHandler(d Deps) nethttp.Handler {
	ok := func(w nethttp.ResponseWriter, _ *nethttp.Request) { w.WriteHeader(204) }
	return build(d, []Route{
		{"GET", "/u", User, ok}, {"GET", "/a", Admin, ok},
		{"GET", "/p", Perm(enum.PermissionOrderReview), ok},
		{"POST", "/hook", Sig, ok}, {"POST", "/write", User, ok},
	})
}

// loader answers authz.Loader from a map.
type loader map[uuid.UUID][]authz.Membership

func (l loader) Memberships(_ context.Context, id uuid.UUID) ([]authz.Membership, error) {
	return l[id], nil
}

func asActor(a authn.Actor) func(*nethttp.Request) (authn.Actor, bool) {
	return func(*nethttp.Request) (authn.Actor, bool) { return a, true }
}

func TestGuards(t *testing.T) {
	user := authn.Actor{UserID: uuid.New()}
	adminNoTotp := authn.Actor{UserID: uuid.New()}
	admin := adminNoTotp
	admin.TotpOK = true
	withRoles := func(a authn.Actor) func(Deps) Deps {
		return func(d Deps) Deps {
			d.Authenticate = asActor(a)
			d.Memberships = loader{adminNoTotp.UserID: {{Role: enum.RoleSuperAdmin}}}
			return d
		}
	}

	cases := []struct {
		name  string
		deps  func(Deps) Deps
		path  string
		want  int
		wcode string
	}{
		{"user anonymous", func(d Deps) Deps { return d }, "/v1/u", 401, "UNAUTHENTICATED"},
		{"user ok", func(d Deps) Deps { d.Authenticate = asActor(user); return d }, "/v1/u", 204, ""},
		{"admin as customer", withRoles(user), "/v1/a", 403, "FORBIDDEN"},
		{"admin without totp", withRoles(adminNoTotp), "/v1/a", 401, "TOTP_REQUIRED"},
		{"admin ok", withRoles(admin), "/v1/a", 204, ""},
		{"admin fails closed without a loader", func(d Deps) Deps { d.Authenticate = asActor(admin); return d }, "/v1/a", 403, "FORBIDDEN"},
		{"perm fails closed", func(d Deps) Deps { d.Authenticate = asActor(user); return d }, "/v1/p", 403, "FORBIDDEN"},
		{"perm anonymous", func(d Deps) Deps { return d }, "/v1/p", 401, "UNAUTHENTICATED"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := do(guardedHandler(c.deps(testDeps())), "GET", c.path, nil, "")
			if w.Code != c.want || (c.wcode != "" && !strings.Contains(w.Body.String(), c.wcode)) {
				t.Fatalf("got %d %q, want %d %s", w.Code, w.Body.String(), c.want, c.wcode)
			}
		})
	}
}

func TestSigRouteFailsClosedWithoutVerifier(t *testing.T) {
	w := do(guardedHandler(testDeps()), "POST", "/v1/hook", nil, "{}")
	if w.Code != 401 {
		t.Fatalf("got %d, want 401", w.Code)
	}
}

// CSRF-01: a state-changing request from a browser needs an allowed Origin and the custom header.
func TestCSRF01(t *testing.T) {
	h := guardedHandler(withUser(testDeps()))
	cases := []struct {
		name string
		hdr  map[string]string
		want int
	}{
		{"no origin, no header", nil, 403},
		{"origin but no header", map[string]string{"Origin": webOrigin}, 403},
		{"header but no origin", map[string]string{"X-Requested-With": "gp-web"}, 403},
		{"evil origin", map[string]string{"Origin": "https://evil.test", "X-Requested-With": "gp-web"}, 403},
		{"null origin", map[string]string{"Origin": "null", "X-Requested-With": "gp-web"}, 403},
		{"good", map[string]string{"Origin": webOrigin, "X-Requested-With": "gp-web"}, 204},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if w := do(h, "POST", "/v1/write", c.hdr, "{}"); w.Code != c.want {
				t.Fatalf("got %d, want %d", w.Code, c.want)
			}
		})
	}
}

func withUser(d Deps) Deps {
	d.Authenticate = asActor(authn.Actor{UserID: uuid.New()})
	return d
}

// A signed route skips CSRF (the signature is its protection) and uses the 16 KB limit.
func TestSigRouteSkipsCSRFAndUsesSmallLimit(t *testing.T) {
	d := testDeps()
	d.VerifySig = func(*nethttp.Request) error { return nil }
	h := guardedHandler(d)
	if w := do(h, "POST", "/v1/hook", nil, "{}"); w.Code != 204 {
		t.Fatalf("signed call got %d, want 204", w.Code)
	}
	big := `{"x":"` + strings.Repeat("a", 17<<10) + `"}`
	if w := do(h, "POST", "/v1/hook", nil, big); w.Code != 413 {
		t.Fatalf("17 KB signed body got %d, want 413", w.Code)
	}
}

// CORS-01 / CORS-02: exact origins only, never *, never reflected, never null.
func TestCORS(t *testing.T) {
	h := New(testDeps())
	w := do(h, "GET", "/v1/health", map[string]string{"Origin": webOrigin}, "")
	if w.Header().Get("Access-Control-Allow-Origin") != webOrigin || w.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("allowed origin headers wrong: %v", w.Header())
	}
	for _, o := range []string{"https://evil.test", "null", webOrigin + ".evil.test", "http://app.example.test"} {
		w := do(h, "GET", "/v1/health", map[string]string{"Origin": o}, "")
		if v := w.Header().Get("Access-Control-Allow-Origin"); v != "" {
			t.Errorf("origin %q was allowed as %q", o, v)
		}
	}
	pre := do(h, "OPTIONS", "/v1/bootstrap", map[string]string{"Origin": webOrigin, "Access-Control-Request-Method": "POST"}, "")
	if pre.Code != 204 || pre.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Errorf("preflight got %d %v", pre.Code, pre.Header())
	}
	bad := do(h, "OPTIONS", "/v1/bootstrap", map[string]string{"Origin": "https://evil.test", "Access-Control-Request-Method": "POST"}, "")
	if bad.Header().Get("Access-Control-Allow-Methods") != "" {
		t.Errorf("preflight from evil origin was answered: %v", bad.Header())
	}
}
