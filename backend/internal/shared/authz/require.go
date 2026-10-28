package authz

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authn"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// Membership is one active role of the caller. ShopID is uuid.Nil for platform
// roles. The slug and name are carried for the "me" object.
type Membership struct {
	ID              uuid.UUID
	Role            enum.Role
	ShopID          uuid.UUID
	ShopSlug        string
	ShopName        string
	CanEditProducts bool
}

// Loader returns the caller's active memberships. The team module implements it.
type Loader interface {
	Memberships(ctx context.Context, userID uuid.UUID) ([]Membership, error)
}

// Has reports whether this membership grants the permission, including the
// conditional ones from the generated map.
func (m Membership) Has(p enum.Permission) bool {
	if RoleHas(m.Role, p) {
		return true
	}
	cond, ok := Condition(m.Role, p)
	return ok && conditionHolds(cond, m)
}

// conditionHolds understands every condition text the contract can produce. An
// unknown text denies, and TestEveryConditionIsUnderstood fails the build so a
// new condition cannot silently lock people out.
func conditionHolds(cond string, m Membership) bool {
	switch cond {
	case "memberships.can_edit_products = true":
		return m.CanEditProducts
	}
	return false
}

// Require decides whether the caller may use a permission and returns the
// database scope for the request (docs/07 §3). shopID is the {shopId} of the
// route, or uuid.Nil for routes without one.
//
//   - Super admin: needs the TOTP-verified session, then runs privileged.
//   - Shop route: no membership for that shop is 404, so shop ids cannot be
//     probed; a membership without the permission is 403.
//   - Route without a shop (driver screens): any membership that holds it.
func Require(a authn.Actor, ms []Membership, shopID uuid.UUID, p enum.Permission) (db.Scope, error) {
	for _, m := range ms {
		if m.Role == enum.RoleSuperAdmin && m.Has(p) {
			if !a.TotpOK {
				return db.Scope{}, apperr.New(apperr.TotpRequired)
			}
			return db.Scope{UserID: a.UserID, ShopID: shopID, Privileged: true, Role: m.Role}, nil
		}
	}
	if shopID == uuid.Nil {
		for _, m := range ms {
			if m.Has(p) {
				// No shop in the scope: a driver sees orders by driver_id, not by shop.
				return db.Scope{UserID: a.UserID, Role: m.Role}, nil
			}
		}
		return db.Scope{}, apperr.New(apperr.Forbidden)
	}
	for _, m := range ms {
		if m.ShopID != shopID {
			continue
		}
		if !m.Has(p) {
			return db.Scope{}, apperr.New(apperr.Forbidden)
		}
		return db.Scope{UserID: a.UserID, ShopID: shopID, Role: m.Role}, nil
	}
	return db.Scope{}, apperr.New(apperr.NotFound)
}

// RequireAdmin is the admin guard: a SUPER_ADMIN membership and a TOTP-verified
// session (AUTHN-05, AUTHZ-07).
func RequireAdmin(a authn.Actor, ms []Membership) (db.Scope, error) {
	for _, m := range ms {
		if m.Role != enum.RoleSuperAdmin {
			continue
		}
		if !a.TotpOK {
			return db.Scope{}, apperr.New(apperr.TotpRequired)
		}
		return db.Scope{UserID: a.UserID, Privileged: true, Role: m.Role}, nil
	}
	return db.Scope{}, apperr.New(apperr.Forbidden)
}

type scopeKey struct{}

// WithScope stores the scope the guard decided on, for the handler.
func WithScope(ctx context.Context, s db.Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, s)
}

// ScopeFrom returns the scope a guard stored. Handlers behind a user, admin or
// perm guard always have one.
func ScopeFrom(ctx context.Context) (db.Scope, bool) {
	s, ok := ctx.Value(scopeKey{}).(db.Scope)
	return s, ok
}

// UserScope is the scope of a signed-in user's own request ("me" routes). It
// fails closed: a request without a user is UNAUTHENTICATED, never an empty scope.
func UserScope(ctx context.Context) (db.Scope, error) {
	s, ok := ScopeFrom(ctx)
	if !ok || s.UserID == uuid.Nil {
		return db.Scope{}, apperr.New(apperr.Unauthenticated)
	}
	return s, nil
}
