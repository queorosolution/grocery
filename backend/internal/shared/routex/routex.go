// Package routex is the one way a handler behind a guard reads its scope, its
// {id} and its body, so no module writes a second copy of these steps.
package routex

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authz"
	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
)

// AdminScope returns the scope the admin guard stored. Admin routes need a
// privileged scope; anything else fails closed.
func AdminScope(r *http.Request) (db.Scope, error) {
	s, ok := authz.ScopeFrom(r.Context())
	if !ok || !s.Privileged {
		return db.Scope{}, apperr.New(apperr.Forbidden)
	}
	return s, nil
}

// ShopScope returns the scope a perm: guard stored for a console shop route. It
// always names the shop; a scope without one fails closed.
func ShopScope(r *http.Request) (db.Scope, error) {
	s, ok := authz.ScopeFrom(r.Context())
	if !ok || s.ShopID == uuid.Nil {
		return db.Scope{}, apperr.New(apperr.Forbidden)
	}
	return s, nil
}

// UserScope returns the scope of a signed-in user's own request. The user comes
// from the guard, never from the request.
func UserScope(r *http.Request) (db.Scope, error) { return authz.UserScope(r.Context()) }

// PathID reads a UUID path value. A malformed one is NOT_FOUND, like an unknown one.
func PathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, apperr.New(apperr.NotFound)
	}
	return id, nil
}

// Run is the code of one route. id is the {id} path value when the route has one.
type Run func(r *http.Request, scope db.Scope, id uuid.UUID) (any, error)

// Admin runs an admin route: scope, the {id} path value (when withID), the JSON
// body (when body is not nil), then run. The result is written as JSON with
// status; a nil result is 204.
func Admin[B any](w http.ResponseWriter, r *http.Request, status int, withID bool, body *B, run Run) {
	serve(w, r, status, AdminScope, withID, body, run)
}

// Console is Admin for the routes of one shop's console, behind a perm: guard.
// The shop is scope.ShopID: it comes from the guard, never from the body.
func Console[B any](w http.ResponseWriter, r *http.Request, status int, withID bool, body *B, run Run) {
	serve(w, r, status, ShopScope, withID, body, run)
}

// User is Admin for the routes of a signed-in user's own data.
func User[B any](w http.ResponseWriter, r *http.Request, status int, withID bool, body *B, run Run) {
	serve(w, r, status, UserScope, withID, body, run)
}

func serve[B any](w http.ResponseWriter, r *http.Request, status int, scopeOf func(*http.Request) (db.Scope, error),
	withID bool, body *B, run Run) {
	scope, err := scopeOf(r)
	var id uuid.UUID
	if err == nil && withID {
		id, err = PathID(r, "id")
	}
	if err == nil && body != nil {
		err = httpx.Decode(r, body)
	}
	var out any
	if err == nil {
		out, err = run(r, scope, id)
	}
	switch {
	case err != nil:
		httpx.Error(w, r, err)
	case out == nil:
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.JSON(w, status, out)
	}
}
