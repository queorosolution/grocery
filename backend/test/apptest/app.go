// Package apptest builds the real API (router, auth, team, database) around a
// fresh Postgres and a local Google, for tests that go through HTTP. Only the
// outside world is faked.
package apptest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"

	"gp/internal/audit"
	"gp/internal/auth"
	"gp/internal/cartclient"
	"gp/internal/catalog"
	"gp/internal/connector"
	"gp/internal/connector/source"
	"gp/internal/delivery"
	"gp/internal/events"
	apihttp "gp/internal/http"
	"gp/internal/notify"
	"gp/internal/pricing"
	"gp/internal/shared/clock"
	"gp/internal/shared/config"
	"gp/internal/shop"
	"gp/internal/team"
	"gp/internal/upload"
	"gp/internal/user"
	"gp/test/fakes"
	"gp/test/testdb"
)

const (
	ClientID   = "test-client-id.apps.example.test"
	WebOrigin  = "https://app.example.test"
	AdminEmail = "boss@example.test"
	DefaultIP  = "198.51.100.1"
	ImgBase    = "https://img.example.test"
)

// Secrets is an in-memory secret store; the preset values are base64 of 32 bytes.
// Everything written through Put is kept, so tests can see what was stored.
type Secrets struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *Secrets) Get(_ context.Context, name string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[name], nil
}

func (s *Secrets) Put(_ context.Context, name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[name] = value
	return nil
}

type App struct {
	T       *testing.T
	Env     *testdb.Env
	OIDC    *fakes.OIDC
	Handler http.Handler
	Deps    apihttp.Deps
	Secrets *Secrets
	// SMS keeps the messages the API tried to send (OTP codes).
	SMS *fakes.Outbox
	// Cart is the fake cart service the API and the price sync talk to.
	Cart *fakes.Cart
	// Events holds the consumers the worker would run; DrainEvents runs them.
	Events *events.Registry
	ips    int
}

func New(t *testing.T) *App {
	t.Helper()
	env := testdb.New(t)
	oidc := fakes.NewOIDC(t)
	cart := fakes.NewCart(t)
	cfg := &config.Config{
		Env: config.EnvDev, APIBaseURL: "https://api.example.test", AllowedOrigins: []string{WebOrigin},
		GoogleClientID: ClientID, SuperAdminEmails: []string{AdminEmail}, PlatformName: "TestShop",
		ImgBaseURL: ImgBase, R2AccountID: "test-account", R2Bucket: "test-bucket", CartAPIBase: cart.BaseURL(),
	}
	sec := &Secrets{values: map[string]string{
		"enc/key":           "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"jwt/key":           "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
		"ip/pepper":         "cGVwcGVycGVwcGVycGVwcGVycGVwcGVycGU=",
		"otp/pepper":        "b3RwcGVwcGVyb3RwcGVwcGVyb3RwcGVwcGVyb3A=",
		"r2/keys":           `{"accessKeyId":"test-access-key","secretAccessKey":"test-secret-key"}`,
		"cart/platform-key": fakes.CartKey,
	}}
	teamSvc := team.NewService(env.DB)
	svc := auth.NewService(cfg, env.DB, sec, auth.NewOIDCVerifier(oidc.Issuer, ClientID), teamSvc)
	shopSvc := shop.NewService(env.DB, sec, true)
	deliverySvc := delivery.NewService(env.DB, shopSvc)
	pricingSvc := pricing.NewService(env.DB, shopSvc)
	shopSvc.SetRepricer(pricingSvc)
	catalogSvc := catalog.NewService(env.DB, shopSvc, deliverySvc, pricingSvc, cfg.ImgBaseURL)
	carts := cartclient.New(cfg.CartAPIBase, sec, cfg.AllowedOrigins).WithoutDelays()
	shopSvc.SetCarts(carts, catalogSvc)
	reg := events.NewRegistry()
	catalog.NewCartSync(shopSvc, carts).Register(reg)
	connectorSvc := connector.NewService(env.DB, sec, localSource(env), cfg.ImgBaseURL)
	uploadSvc := upload.NewService(upload.NewR2(cfg.R2AccountID, cfg.R2Bucket, sec), cfg.ImgBaseURL)
	sms := &fakes.Outbox{}
	userSvc := user.NewService(env.DB, sec, fakes.Turnstile{}, notify.New(sms), deliverySvc, cfg.PlatformName)
	deps := apihttp.Deps{
		Config: cfg, Auth: auth.NewHandler(svc, cfg), Team: team.NewHandler(teamSvc), Memberships: teamSvc,
		Shop:      shop.NewHandler(shopSvc),
		Delivery:  delivery.NewHandler(deliverySvc),
		User:      user.NewHandler(userSvc, svc),
		Catalog:   catalog.NewHandler(catalogSvc),
		Pricing:   pricing.NewHandler(pricingSvc),
		Upload:    upload.NewHandler(uploadSvc, teamSvc),
		Connector: connector.NewHandler(connectorSvc),
		Audit:     audit.NewHandler(env.DB),
	}
	return &App{T: t, Env: env, OIDC: oidc, Handler: apihttp.New(deps), Deps: deps, Secrets: sec, SMS: sms, Cart: cart, Events: reg}
}

// Call sends a browser-like request: the CSRF headers are always present.
func (a *App) Call(method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	return a.CallFrom(DefaultIP, method, path, body, cookies...)
}

func (a *App) CallFrom(ip, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Origin", WebOrigin)
	r.Header.Set("X-Requested-With", "gp-web")
	r.Header.Set("CF-Connecting-IP", ip)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	a.Handler.ServeHTTP(w, r)
	return w
}

// SignIn runs POST /auth/google with a valid token for the identity.
func (a *App) SignIn(c fakes.Claims) *httptest.ResponseRecorder {
	a.T.Helper()
	return a.SignInFrom(DefaultIP, c)
}

func (a *App) SignInFrom(ip string, c fakes.Claims) *httptest.ResponseRecorder {
	a.T.Helper()
	return a.signIn(ip, c, "")
}

// SignInWithInvite signs in and hands over an invite token.
func (a *App) SignInWithInvite(c fakes.Claims, invite string) *httptest.ResponseRecorder {
	a.T.Helper()
	return a.signIn(DefaultIP, c, invite)
}

func (a *App) signIn(ip string, c fakes.Claims, invite string) *httptest.ResponseRecorder {
	body := map[string]string{"idToken": a.OIDC.Token(a.T, ClientID, c)}
	if invite != "" {
		body["inviteToken"] = invite
	}
	raw, _ := json.Marshal(body)
	return a.CallFrom(ip, http.MethodPost, "/v1/auth/google", string(raw))
}

// Session is a signed-in browser: its cookies and who it is.
type Session struct {
	Access, Refresh *http.Cookie
	Me              auth.Me
}

// Cookies are what the browser sends on API calls.
func (s Session) Cookies() []*http.Cookie { return []*http.Cookie{s.Access, s.Refresh} }

// SignedIn signs in from a fresh address (so the per-IP sign-in limit never
// gets in the way of a test that needs many users) and fails unless it works.
func (a *App) SignedIn(c fakes.Claims) Session {
	a.T.Helper()
	a.ips++
	w := a.SignInFrom(fmt.Sprintf("192.0.2.%d", a.ips%250+1), c)
	if w.Code != http.StatusOK {
		a.T.Fatalf("sign-in = %d %s", w.Code, w.Body)
	}
	got := Cookies(w)
	return Session{Access: got[auth.AccessCookie], Refresh: got[auth.RefreshCookie], Me: Decode[auth.Me](a.T, w)}
}

// Cookies returns the cookies a response set, by name.
func Cookies(w *httptest.ResponseRecorder) map[string]*http.Cookie {
	out := map[string]*http.Cookie{}
	for _, c := range w.Result().Cookies() {
		out[c.Name] = c
	}
	return out
}

func Decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("bad JSON %q: %v", w.Body.String(), err)
	}
	return v
}

// Count runs a SELECT count(*) as the database owner (it skips row security).
func (a *App) Count(query string, args ...any) int {
	a.T.Helper()
	var n int
	if err := a.Env.Owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		a.T.Fatal(err)
	}
	return n
}

// Exec runs a statement as the database owner, to set up or change state.
func (a *App) Exec(query string, args ...any) {
	a.T.Helper()
	if _, err := a.Env.Owner.Exec(context.Background(), query, args...); err != nil {
		a.T.Fatal(err)
	}
}

// SeedShop inserts a shop row with its default settings (as shop.Create does)
// and returns its id.
func (a *App) SeedShop(slug string) uuid.UUID {
	a.T.Helper()
	var id uuid.UUID
	err := a.Env.Owner.QueryRow(context.Background(),
		`INSERT INTO shops (slug, name, lat, lng) VALUES ($1, $2, 6.9271, 79.8612) RETURNING id`, slug, "Shop "+slug).Scan(&id)
	if err != nil {
		a.T.Fatal(err)
	}
	a.Exec(`INSERT INTO shop_settings (shop_id) VALUES ($1)`, id)
	return id
}

// Grant gives a signed-in user a role, as an admin would through an invite.
func (a *App) Grant(userID uuid.UUID, shopID *uuid.UUID, role string) {
	a.T.Helper()
	scope := "SHOP"
	if shopID == nil {
		scope = "PLATFORM"
	}
	a.Exec(`INSERT INTO memberships (user_id, scope, shop_id, role) VALUES ($1, $2, $3, $4)`, userID, scope, shopID, role)
}

// Admin signs in as the super admin and passes the authenticator check, so the
// session can use admin routes.
func (a *App) Admin() Session {
	a.T.Helper()
	s := a.SignedIn(fakes.Claims{Sub: "boss", Email: AdminEmail})
	w := a.Call(http.MethodPost, "/v1/auth/totp/setup", "", s.Cookies()...)
	if w.Code != http.StatusOK {
		a.T.Fatalf("totp setup = %d %s", w.Code, w.Body)
	}
	var setup struct {
		URI string `json:"otpauthUri"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &setup)
	u, err := url.Parse(setup.URI)
	if err != nil {
		a.T.Fatal(err)
	}
	code, err := totp.GenerateCode(u.Query().Get("secret"), clock.Now())
	if err != nil {
		a.T.Fatal(err)
	}
	w = a.Call(http.MethodPost, "/v1/auth/totp/verify", `{"code":"`+code+`"}`, s.Cookies()...)
	if w.Code != http.StatusOK {
		a.T.Fatalf("totp verify = %d %s", w.Code, w.Body)
	}
	s.Access = Cookies(w)[auth.AccessCookie]
	s.Me = Decode[auth.Me](a.T, w)
	return s
}

// DrainEvents runs the consumers over every event written so far, in order, as
// the worker would after the queue delivers them. Events already processed are
// skipped, so it is safe to call again. It returns the first failure.
func (a *App) DrainEvents() error {
	ctx := context.Background()
	rows, err := a.Env.Owner.Query(ctx, `
		SELECT id, type, shop_id, coalesce(entity_id, ''), payload, created_at FROM outbox ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	var msgs []events.Message
	for rows.Next() {
		var m events.Message
		if err := rows.Scan(&m.ID, &m.Type, &m.ShopID, &m.EntityID, &m.Data, &m.At); err != nil {
			rows.Close()
			return err
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, m := range msgs {
		if err := a.Events.Dispatch(ctx, a.Env.DB, m); err != nil {
			return err
		}
	}
	return nil
}

// localSource opens the test database as if it were a shop's database: the
// local address and port are allowed and TLS is off. The production policy
// would refuse both, which source's own tests check.
func localSource(env *testdb.Env) *source.Postgres {
	u, err := url.Parse(env.OwnerURL)
	if err != nil {
		panic(err)
	}
	port, _ := strconv.Atoi(u.Port())
	return source.NewPostgres(source.Options{Policy: source.Policy{AllowLocal: true, Port: port}, PlaintextForTest: true})
}

// SourceString is a connection string for the test database, owner login, in
// the form the connector accepts.
func (a *App) SourceString() string {
	a.T.Helper()
	u, err := url.Parse(a.Env.OwnerURL)
	if err != nil {
		a.T.Fatal(err)
	}
	u.RawQuery = url.Values{"sslmode": {"require"}}.Encode()
	return u.String()
}
