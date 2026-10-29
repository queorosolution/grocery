package auth

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/authz"
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
func (s *Service) loadMe(ctx context.Context, tx *db.Tx, userID uuid.UUID, totpOK bool) (Me, error) {
	u, ok, err := userByID(ctx, tx, userID)
	if err != nil {
		return Me{}, err
	}
	if !ok || u.Status != enum.UserStatusActive {
		return Me{}, apperr.New(apperr.Unauthenticated)
	}
	ms, err := s.team.MembershipsTx(ctx, tx, userID)
	if err != nil {
		return Me{}, err
	}
	me := Me{
		ID: u.ID, Name: u.Name, Email: u.Email,
		PhoneVerified: u.PhoneVerified, NeedsOnboarding: !u.PhoneVerified,
		Roles: roleInfos(ms), AdminTotp: AdminTotpNotRequired,
	}
	if u.Phone != nil {
		m := maskPhone(*u.Phone)
		me.PhoneMasked = &m
	}
	if isSuperAdmin(ms) {
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

func roleInfos(ms []authz.Membership) []RoleInfo {
	out := make([]RoleInfo, 0, len(ms))
	for _, m := range ms {
		r := RoleInfo{Role: m.Role, CanEditProducts: m.Has(enum.PermissionProductEdit)}
		if m.ShopID != uuid.Nil {
			id, slug, name := m.ShopID, m.ShopSlug, m.ShopName
			r.ShopID, r.ShopSlug, r.ShopName = &id, &slug, &name
		}
		out = append(out, r)
	}
	return out
}

func isSuperAdmin(ms []authz.Membership) bool {
	for _, m := range ms {
		if m.Role == enum.RoleSuperAdmin {
			return true
		}
	}
	return false
}
