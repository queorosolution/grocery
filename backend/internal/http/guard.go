package http

import (
	"fmt"
	nethttp "net/http"
	"strings"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/authz"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/httpx"
)

// Guard says who may call a route. It is written as text in the route table
// (docs/07 section 3): public, user, admin, sig, or perm:<permission>.
type Guard string

const (
	Public Guard = "public"
	User   Guard = "user"
	Admin  Guard = "admin"
	Sig    Guard = "sig"
)

// Perm builds a permission guard from a generated permission constant.
func Perm(p enum.Permission) Guard { return Guard("perm:" + string(p)) }

// Validate fails for anything that is not a known guard, so a typo in the route
// table stops the build test and never ships as an open route.
func (g Guard) Validate() error {
	switch g {
	case Public, User, Admin, Sig:
		return nil
	}
	if name, ok := strings.CutPrefix(string(g), "perm:"); ok && enum.Permission(name).Valid() {
		return nil
	}
	return fmt.Errorf("unknown guard %q", g)
}

// SigVerifier checks a signed server-to-server call (payment, webhooks). Nil
// denies every sig route.
type SigVerifier func(r *nethttp.Request) error

type guards struct {
	memberships authz.Loader
	verifySig   SigVerifier
}

// wrap puts the route's guard in front of its handler. A guard that fails never
// reaches the handler, and the matched pattern is recorded for the access log.
// The scope a guard decides on travels in the request context (authz.ScopeFrom).
func (g guards) wrap(rt Route, h nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		httpx.SetRoute(r.Context(), rt.Method+" "+rt.Pattern)
		r, err := g.check(rt, r)
		if err != nil {
			httpx.Error(w, r, err)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (g guards) check(rt Route, r *nethttp.Request) (*nethttp.Request, error) {
	switch rt.Guard {
	case Public:
		return r, nil
	case Sig:
		if g.verifySig == nil {
			return r, apperr.New(apperr.Unauthenticated)
		}
		return r, g.verifySig(r)
	}
	actor, ok := authn.From(r.Context())
	if !ok {
		return r, apperr.New(apperr.Unauthenticated)
	}
	if rt.Guard == User {
		return withScope(r, db.Scope{UserID: actor.UserID, Role: enum.RoleCustomer}), nil
	}
	if g.memberships == nil {
		return r, apperr.New(apperr.Forbidden) // fail closed until the team module is wired
	}
	ms, err := g.memberships.Memberships(r.Context(), actor.UserID)
	if err != nil {
		return r, err
	}
	if rt.Guard == Admin {
		scope, err := authz.RequireAdmin(actor, ms)
		return withScope(r, scope), err
	}
	shopID, err := shopIDOf(rt, r)
	if err != nil {
		return r, err
	}
	name, _ := strings.CutPrefix(string(rt.Guard), "perm:")
	scope, err := authz.Require(actor, ms, shopID, enum.Permission(name))
	return withScope(r, scope), err
}

// shopIDOf reads {shopId} from routes that have one. A malformed id is the same
// 404 as a shop the caller does not belong to.
func shopIDOf(rt Route, r *nethttp.Request) (uuid.UUID, error) {
	if !strings.Contains(rt.Pattern, "{shopId}") {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(r.PathValue("shopId"))
	if err != nil {
		return uuid.Nil, apperr.New(apperr.NotFound)
	}
	return id, nil
}

func withScope(r *nethttp.Request, s db.Scope) *nethttp.Request {
	return r.WithContext(authz.WithScope(r.Context(), s))
}
