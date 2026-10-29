// Package auth is Google sign-in, sessions and the "me" object (docs/06 §4,
// docs/10 §4.1). It owns the users, sessions and admin_totp rows; other modules
// read who the caller is from authn.Actor and never touch these tables.
package auth

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
)

const (
	// Audience is the "aud" of our own access tokens, so a token minted for
	// another service can never be replayed here.
	Audience = "gp-web"

	AccessTTL  = 15 * time.Minute
	RefreshTTL = 30 * 24 * time.Hour

	// Sign-in is limited per IP before the Google token is checked, and per
	// Google account after (docs/06 §1, AUTHN-01).
	signInLimit  = 10
	signInWindow = 10 * time.Minute
)

// AdminTotp tells the client which admin screen to show next.
type AdminTotp string

const (
	AdminTotpNotRequired AdminTotp = "NOT_REQUIRED"
	AdminTotpSetupNeeded AdminTotp = "SETUP_NEEDED"
	AdminTotpCodeNeeded  AdminTotp = "CODE_NEEDED"
	AdminTotpOK          AdminTotp = "OK"
)

// Me is the signed-in user as the client sees them (docs/06 §3).
type Me struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	Email           string     `json:"email"`
	PhoneMasked     *string    `json:"phoneMasked"`
	PhoneVerified   bool       `json:"phoneVerified"`
	NeedsOnboarding bool       `json:"needsOnboarding"`
	Roles           []RoleInfo `json:"roles"`
	AdminTotp       AdminTotp  `json:"adminTotp"`
}

// RoleInfo is one active membership. Shop fields are empty for platform roles.
type RoleInfo struct {
	Role     enum.Role  `json:"role"`
	ShopID   *uuid.UUID `json:"shopId"`
	ShopSlug *string    `json:"shopSlug"`
	ShopName *string    `json:"shopName"`
	// CanEditProducts tells the console whether to offer product edits: always for an owner,
	// for staff only when the owner switched it on. The API checks it again on every edit.
	CanEditProducts bool `json:"canEditProducts"`
}

// GoogleIdentity is what we trust from a verified Google ID token.
type GoogleIdentity struct {
	Sub   string
	Email string
	Name  string
}

// Tokens are the two secrets the handler turns into cookies.
type Tokens struct {
	Access         string
	AccessExpires  time.Time
	Refresh        string
	RefreshExpires time.Time
}

// ClientInfo is what we keep about a device to help the user spot a strange
// session. The IP is only ever stored as a keyed hash.
type ClientInfo struct {
	IP        string
	UserAgent string
}
