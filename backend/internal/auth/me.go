package auth

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// maskPhone shows enough to recognise a number, "+94771234567" -> "077•••4567".
func maskPhone(e164 string) string {
	local := "0" + strings.TrimPrefix(e164, "+94")
	if len(local) != 10 {
		return "•••"
	}
	return local[:3] + "•••" + local[6:]
}

// loadMe builds the client view. totpOK is whether this session passed the
// authenticator check; it only matters for super admins.
func loadMe(ctx context.Context, tx *db.Tx, userID uuid.UUID, totpOK bool) (Me, error) {
	u, ok, err := userByID(ctx, tx, userID)
	if err != nil {
		return Me{}, err
	}
	if !ok || u.Status != statusActive {
		return Me{}, apperr.New(apperr.Unauthenticated)
	}
	roles, err := rolesOf(ctx, tx, userID)
	if err != nil {
		return Me{}, err
	}
	me := Me{
		ID: u.ID, Name: u.Name, Email: u.Email,
		PhoneVerified: u.PhoneVerified, NeedsOnboarding: !u.PhoneVerified,
		Roles: roles, AdminTotp: AdminTotpNotRequired,
	}
	if u.Phone != nil {
		m := maskPhone(*u.Phone)
		me.PhoneMasked = &m
	}
	if hasRole(roles, enum.RoleSuperAdmin) {
		enabled, err := totpEnabled(ctx, tx, userID)
		if err != nil {
			return Me{}, err
		}
		switch {
		case !enabled:
			me.AdminTotp = AdminTotpSetupNeeded
		case totpOK:
			me.AdminTotp = AdminTotpOK
		default:
			me.AdminTotp = AdminTotpCodeNeeded
		}
	}
	return me, nil
}
