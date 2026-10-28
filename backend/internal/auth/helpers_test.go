package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gp/internal/auth"
	apihttp "gp/internal/http"
	"gp/internal/shared/config"
	"gp/test/fakes"
	"gp/test/testdb"
)

const (
	webOrigin  = "https://app.example.test"
	adminEmail = "boss@example.test"
)

// staticSecrets answers secrets.Getter from a map. The keys are valid base64
// of 32 bytes.
type staticSecrets map[string]string

func (s staticSecrets) Get(_ context.Context, name string) (string, error) { return s[name], nil }

// app is the real router, service and database; only Google is local.
type app struct {
	t       *testing.T
	env     *testdb.Env
	oidc    *fakes.OIDC
	handler http.Handler
}

func newApp(t *testing.T) *app {
	t.Helper()
	env := testdb.New(t)
	oidc := fakes.NewOIDC(t)
	cfg := &config.Config{
		Env: config.EnvDev, APIBaseURL: "https://api.example.test", AllowedOrigins: []string{webOrigin},
		GoogleClientID: clientID, SuperAdminEmails: []string{adminEmail},
	}
	sec := staticSecrets{
		"jwt/key":   "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"ip/pepper": "cGVwcGVycGVwcGVycGVwcGVycGVwcGVycGVwcGVycGU=",
	}
	svc := auth.NewService(cfg, env.DB, sec, auth.NewOIDCVerifier(oidc.Issuer, clientID))
	h := apihttp.New(apihttp.Deps{Config: cfg, Auth: auth.NewHandler(svc, cfg)})
	return &app{t: t, env: env, oidc: oidc, handler: h}
}

// call sends a browser-like request: the CSRF headers are always present.
func (a *app) call(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return a.callFrom("198.51.100.1", method, path, body, cookies...)
}

func (a *app) callFrom(ip, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Origin", webOrigin)
	r.Header.Set("X-Requested-With", "gp-web")
	r.Header.Set("CF-Connecting-IP", ip)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.handler.ServeHTTP(w, r)
	return w
}

// signIn runs POST /auth/google with a valid token for the identity.
func (a *app) signIn(c fakes.Claims) *httptest.ResponseRecorder {
	a.t.Helper()
	return a.signInFrom("198.51.100.1", c)
}

func (a *app) signInFrom(ip string, c fakes.Claims) *httptest.ResponseRecorder {
	a.t.Helper()
	body, _ := json.Marshal(map[string]string{"idToken": a.oidc.Token(a.t, clientID, c)})
	return a.callFrom(ip, http.MethodPost, "/v1/auth/google", string(body))
}

// cookies returns the cookies a response set, by name.
func cookies(w *httptest.ResponseRecorder) map[string]*http.Cookie {
	out := map[string]*http.Cookie{}
	for _, c := range w.Result().Cookies() {
		out[c.Name] = c
	}
	return out
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("bad JSON %q: %v", w.Body.String(), err)
	}
	return v
}

func (a *app) count(query string, args ...any) int {
	a.t.Helper()
	var n int
	if err := a.env.Owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		a.t.Fatal(err)
	}
	return n
}
