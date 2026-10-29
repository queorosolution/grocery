// Package http builds the API router: the route table, guards and the
// middleware chain from docs/07 section 3. Modules add one line per route.
package http

import (
	nethttp "net/http"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/config"
	"gp/internal/shared/httpx"
)

// Base is the path prefix of every API route.
const Base = "/v1"

// Deps is everything the router needs. Optional fields fail closed when nil.
type Deps struct {
	Config       *config.Config
	Authenticate func(*nethttp.Request) (authn.Actor, bool)
	Authorize    Authorizer
	VerifySig    SigVerifier
}

// New returns the full API handler with the middleware chain applied.
func New(d Deps) nethttp.Handler {
	return build(d, routes(d))
}

func build(d Deps, table []Route) nethttp.Handler {
	g := guards{authorize: d.Authorize, verifySig: d.VerifySig}
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
		httpx.Authn(d.Authenticate),
	)
}
