// Package adminapi is the one way an admin route reads its scope, its {id} and
// its body, so no module writes a second copy of these steps.
package adminapi

import (
	"net/http"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authz"
	"gp/internal/shared/db"
	"gp/internal/shared/httpx"
)

// Scope returns the scope the admin guard stored. Admin routes need a
// privileged scope; anything else fails closed.
func Scope(r *http.Request) (db.Scope, error) {
	s, ok := authz.ScopeFrom(r.Context())
	if !ok || !s.Privileged {
		return db.Scope{}, apperr.New(apperr.Forbidden)
	}
	return s, nil
}

// PathID reads a UUID path value. A malformed one is NOT_FOUND, like an unknown one.
func PathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, apperr.New(apperr.NotFound)
	}
	return id, nil
}

// Handle runs the common steps of an admin route: scope, the {id} path value
// (when withID), the JSON body (when body is not nil), then run. The result is
// written as JSON with status.
func Handle[B any](w http.ResponseWriter, r *http.Request, status int, withID bool, body *B,
	run func(r *http.Request, scope db.Scope, id uuid.UUID) (any, error)) {
	scope, err := Scope(r)
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
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, status, out)
}
