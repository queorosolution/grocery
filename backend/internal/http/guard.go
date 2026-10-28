package http

import (
	"fmt"
	nethttp "net/http"
	"strings"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
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

// Authorizer answers shop-permission questions. The real one arrives with the
// team module (P08); until then a nil Authorizer denies every perm: route.
type Authorizer func(r *nethttp.Request, a authn.Actor, p enum.Permission) error

// SigVerifier checks a signed server-to-server call (payment, webhooks). Nil
// denies every sig route.
type SigVerifier func(r *nethttp.Request) error

type guards struct {
	authorize Authorizer
	verifySig SigVerifier
}

// wrap puts the route's guard in front of its handler. A guard that fails never
// reaches the handler, and the matched pattern is recorded for the access log.
func (g guards) wrap(rt Route, h nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		httpx.SetRoute(r.Context(), rt.Method+" "+rt.Pattern)
		if err := g.check(rt.Guard, r); err != nil {
			httpx.Error(w, r, err)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func (g guards) check(guard Guard, r *nethttp.Request) error {
	if guard == Public {
		return nil
	}
	if guard == Sig {
		if g.verifySig == nil {
			return apperr.New(apperr.Unauthenticated)
		}
		return g.verifySig(r)
	}
	actor, ok := authn.From(r.Context())
	if !ok {
		return apperr.New(apperr.Unauthenticated)
	}
	switch {
	case guard == User:
		return nil
	case guard == Admin:
		if !actor.HasRole(enum.RoleSuperAdmin) {
			return apperr.New(apperr.Forbidden)
		}
		if !actor.TotpOK {
			return apperr.New(apperr.TotpRequired)
		}
		return nil
	}
	name, _ := strings.CutPrefix(string(guard), "perm:")
	if g.authorize == nil {
		return apperr.New(apperr.Forbidden)
	}
	return g.authorize(r, actor, enum.Permission(name))
}
