// Package http builds the API router: the route table, guards and the
// middleware chain from docs/07 section 3. Modules add one line per route.
package http

import (
	nethttp "net/http"

	"gp/internal/audit"
	"gp/internal/auth"
	"gp/internal/catalog"
	"gp/internal/connector"
	"gp/internal/coupon"
	"gp/internal/delivery"
	"gp/internal/order"
	"gp/internal/pricing"
	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/authz"
	"gp/internal/shared/config"
	"gp/internal/shared/httpx"
	"gp/internal/shop"
	"gp/internal/team"
	"gp/internal/upload"
	"gp/internal/user"
)

// Base is the path prefix of every API route.
const Base = "/v1"

// Deps is everything the router needs. Optional fields fail closed when nil.
type Deps struct {
	Config *config.Config
	// Auth serves sign-in, sessions and "me". Nil means no sign-in: only public
	// routes work, and every guarded route answers 401.
	Auth *auth.Handler
	// Team serves the shop team and admin invites. Nil means no such routes.
	Team *team.Handler
	// Shop serves shops, settings, hours and payment accounts; Audit the log.
	Shop *shop.Handler
	// Delivery serves fee rules and delivery slots.
	Delivery *delivery.Handler
	// User serves the signed-in customer's name, phone and addresses.
	User *user.Handler
	// Catalog serves categories, products and a shop's own products; Pricing the
	// category markups; Upload the image upload links.
	Catalog *catalog.Handler
	Pricing *pricing.Handler
	Upload  *upload.Handler
	// Order serves the customer's bills and orders.
	Order *order.Handler
	// Coupon serves the admin's coupons.
	Coupon *coupon.Handler
	// Connector serves the shop database connector (admin only).
	Connector    *connector.Handler
	Audit        *audit.Handler
	Authenticate func(*nethttp.Request) (authn.Actor, bool)
	// Memberships loads the caller's roles for admin and perm: routes. Nil
	// denies them all.
	Memberships authz.Loader
	VerifySig   SigVerifier
}

// New returns the full API handler with the middleware chain applied.
func New(d Deps) nethttp.Handler {
	return build(d, Routes(d))
}

func build(d Deps, table []Route) nethttp.Handler {
	authenticate := d.Authenticate
	if authenticate == nil && d.Auth != nil {
		authenticate = d.Auth.Authenticate
	}
	g := guards{memberships: d.Memberships, verifySig: d.VerifySig}
	mux := nethttp.NewServeMux()
	byPattern := make(map[string]Route, len(table))
	for _, rt := range table {
		key := rt.Method + " " + Base + rt.Pattern
		byPattern[key] = rt
		mux.Handle(key, g.wrap(rt, rt.Handler))
	}
	mux.HandleFunc("/", func(w nethttp.ResponseWriter, r *nethttp.Request) {
		httpx.Error(w, r, apperr.New(apperr.NotFound))
	})

	// Webhook-style routes skip CSRF and get the smaller body limit.
	isSigned := func(r *nethttp.Request) bool {
		_, pattern := mux.Handler(r)
		rt, ok := byPattern[pattern]
		return ok && rt.Guard == Sig
	}
	return httpx.Chain(mux,
		httpx.WithRequestID(),
		httpx.SecurityHeaders(),
		httpx.Recover(),
		httpx.AccessLog(),
		httpx.BodyLimit(isSigned),
		httpx.CORSAndCSRF(d.Config.AllowedOrigins, isSigned),
		httpx.Authn(authenticate),
	)
}
