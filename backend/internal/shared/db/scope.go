package db

import (
	"github.com/google/uuid"

	"gp/internal/shared/enum"
)

// Scope says who a transaction runs for. Postgres row-level security reads it
// (docs/05 section 5), so a query that forgets a filter still cannot see other
// people's rows. The route guard builds it; services pass it on unchanged.
type Scope struct {
	UserID     uuid.UUID // zero for public requests and system jobs
	ShopID     uuid.UUID // set only on console shop routes, after the membership check
	Privileged bool      // super admin (TOTP passed) or a system job
	Role       enum.Role // role used for this request, for audit and transitions
}

// Public is the scope of a request with no login.
func Public() Scope { return Scope{} }

// System is the scope of workers, timers, sync, the payment callback and auth.
func System() Scope { return Scope{Privileged: true} }

// settings returns the three values for set_config. An empty string means "not set".
func (s Scope) settings() (user, shop, privileged string) {
	if s.UserID != uuid.Nil {
		user = s.UserID.String()
	}
	if s.ShopID != uuid.Nil {
		shop = s.ShopID.String()
	}
	if s.Privileged {
		privileged = "on"
	}
	return user, shop, privileged
}
