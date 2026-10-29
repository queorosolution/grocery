package http

import (
	nethttp "net/http"
)

// Route is one line of the route table. Every route has a guard; a route
// without one fails TestEveryRouteHasValidGuard.
type Route struct {
	Method  string
	Pattern string
	Guard   Guard
	Handler nethttp.HandlerFunc
}

// routes lists every API route. Patterns are relative to the /v1 base.
func routes(d Deps) []Route {
	return []Route{
		{nethttp.MethodGet, "/health", Public, health},
		{nethttp.MethodGet, "/bootstrap", Public, bootstrap(d)},
	}
}
