// Package authn carries who is calling. The sign-in code (P07) fills it; the
// router guards and handlers only read it.
package authn

import (
	"context"

	"github.com/google/uuid"
)

// Actor is a signed-in user as the access token describes them. Roles are not
// in the token: a role change must act at once, so the guards load memberships
// from the database for the routes that need them.
type Actor struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	// TotpOK is true when this session passed the authenticator-app check.
	TotpOK bool
}

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
