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
	table := []Route{
		{nethttp.MethodGet, "/health", Public, health},
		{nethttp.MethodGet, "/bootstrap", Public, bootstrap(d)},
	}
	if d.Auth != nil {
		table = append(table,
			Route{nethttp.MethodPost, "/auth/google", Public, d.Auth.Google},
			Route{nethttp.MethodPost, "/auth/refresh", Public, d.Auth.Refresh},
			Route{nethttp.MethodPost, "/auth/logout", User, d.Auth.Logout},
			Route{nethttp.MethodGet, "/me", User, d.Auth.Me},
		)
	}
	return table
}
