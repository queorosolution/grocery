// Package authn carries who is calling. The sign-in code (P07) fills it; the
// router guards and handlers only read it.
package authn

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
)

// Actor is a signed-in user as the access token describes them.
type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Roles     []enum.Role
	// TotpOK is true when this session passed the authenticator-app check.
	TotpOK bool
}

func (a Actor) HasRole(r enum.Role) bool { return slices.Contains(a.Roles, r) }

type ctxKey struct{}

// With returns a context that carries the actor.
func With(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

// From returns the actor, or false for an anonymous request.
func From(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(ctxKey{}).(Actor)
	return a, ok
}
