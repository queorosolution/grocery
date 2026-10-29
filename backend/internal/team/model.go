// Package team owns memberships and invites: who may act for which shop
// (docs/06 §8, §10, docs/10 §4.1). Every query filters by shop_id itself,
// because invites are privileged-only under row-level security and so the
// module runs in the system scope; the route guard has already proven the
// caller belongs to the shop.
package team

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
)

// InviteTTL is how long an invite link works (FR-AUTH-05).
const InviteTTL = 7 * 24 * time.Hour

type Member struct {
	ID              uuid.UUID `json:"id"`
	UserID          uuid.UUID `json:"userId"`
	Name            string    `json:"name"`
	Email           string    `json:"email"`
	Role            enum.Role `json:"role"`
	CanEditProducts bool      `json:"canEditProducts"`
}

type Invite struct {
	ID              uuid.UUID  `json:"id"`
	Email           string     `json:"email"`
	ShopID          *uuid.UUID `json:"shopId"`
	Role            enum.Role  `json:"role"`
	CanEditProducts bool       `json:"canEditProducts"`
	ExpiresAt       time.Time  `json:"expiresAt"`
}

// Team is the owner's view of a shop.
type Team struct {
	Members []Member `json:"members"`
	Invites []Invite `json:"invites"`
}

// Issued is returned once when an invite is created. There is no e-mail sender
// in the platform, so the token is handed to the inviter, who sends the link.
// Only its hash is stored.
type Issued struct {
	Invite Invite `json:"invite"`
	Token  string `json:"token"`
}

// InviteInput is the validated request for a new invite.
type InviteInput struct {
	Email           string
	Role            enum.Role
	CanEditProducts bool
}

// ChangeInput is a partial update of a membership; nil means "leave as is".
type ChangeInput struct {
	Role            *enum.Role
	CanEditProducts *bool
}
