package team

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/authz"
	"gp/internal/shared/clock"
	"gp/internal/shared/crypt"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/ids"
	"gp/internal/shared/validate"
)

type Service struct{ db *db.DB }

func NewService(d *db.DB) *Service { return &Service{db: d} }

// Memberships implements authz.Loader for the route guards. It reads fresh on
// every call, so removing a role takes effect on the next request.
func (s *Service) Memberships(ctx context.Context, userID uuid.UUID) ([]authz.Membership, error) {
	var out []authz.Membership
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		out, err = membershipsOf(ctx, tx, userID)
		return err
	})
	return out, err
}

// MembershipsTx is the same inside a caller's transaction (sign-in, "me").
func (s *Service) MembershipsTx(ctx context.Context, tx *db.Tx, userID uuid.UUID) ([]authz.Membership, error) {
	return membershipsOf(ctx, tx, userID)
}

// consoleRoles are the roles an owner may hand out. Owners are created by the
// platform admin only (AUTHZ-04).
func consoleRoles(r enum.Role) bool { return r == enum.RoleShopStaff || r == enum.RoleShopDriver }

// List returns the members and pending invites of a shop.
func (s *Service) List(ctx context.Context, shopID uuid.UUID) (Team, error) {
	var t Team
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		if t.Members, err = membersOf(ctx, tx, shopID); err != nil {
			return err
		}
		t.Invites, err = pendingInvites(ctx, tx, shopID)
		return err
	})
	return t, err
}

// Invite creates a staff or driver invite for a shop (owner or admin).
func (s *Service) Invite(ctx context.Context, scope db.Scope, shopID uuid.UUID, in InviteInput) (Issued, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if err := validate.New().
		Email("email", in.Email).
		Check("role", consoleRoles(in.Role), "invalid").
		Err(); err != nil {
		return Issued{}, err
	}
	if in.Role != enum.RoleShopStaff {
		in.CanEditProducts = false // only staff carry the flag
	}
	return s.issue(ctx, scope, &shopID, in)
}

// AdminInvite creates any invite, including owners and platform drivers.
func (s *Service) AdminInvite(ctx context.Context, scope db.Scope, shopID *uuid.UUID, in InviteInput) (Issued, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	v := validate.New().Email("email", in.Email)
	switch in.Role {
	case enum.RolePlatformDriver:
		v.Check("shopId", shopID == nil, "not_allowed")
	case enum.RoleShopOwner, enum.RoleShopStaff, enum.RoleShopDriver:
		v.Check("shopId", shopID != nil, "required")
	default:
		v.Check("role", false, "invalid")
	}
	if err := v.Err(); err != nil {
		return Issued{}, err
	}
	if in.Role != enum.RoleShopStaff {
		in.CanEditProducts = false
	}
	return s.issue(ctx, scope, shopID, in)
}

func (s *Service) issue(ctx context.Context, scope db.Scope, shopID *uuid.UUID, in InviteInput) (Issued, error) {
	token := ids.Token()
	var inv Invite
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		if shopID != nil {
			ok, err := shopExists(ctx, tx, *shopID)
			if err != nil {
				return err
			}
			if !ok {
				return validate.New().Check("shopId", false, "unknown").Err()
			}
		}
		inv, err = insertInvite(ctx, tx, newInvite{
			Email: in.Email, ShopID: shopID, Role: in.Role, CanEditProducts: in.CanEditProducts,
			TokenHash: crypt.HashToken(token), InvitedBy: scope.UserID, ExpiresAt: clock.Now().Add(InviteTTL),
		})
		if err != nil {
			return err
		}
		// The token is not audited: only its hash is stored anywhere.
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "invite.create", Entity: "invite", EntityID: inv.ID.String(), ShopID: shopOf(shopID), After: inv,
		})
	})
	if err != nil {
		return Issued{}, err
	}
	return Issued{Invite: inv, Token: token}, nil
}

// RevokeInvite cancels a pending invite (admin).
func (s *Service) RevokeInvite(ctx context.Context, scope db.Scope, id uuid.UUID) error {
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		ok, err := revokeInvite(ctx, tx, id)
		if err != nil {
			return err
		}
		if !ok {
			return apperr.New(apperr.NotFound)
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "invite.revoke", Entity: "invite", EntityID: id.String()})
	})
}

// Accept turns an invite into a membership inside the sign-in transaction. An
// unknown, used, revoked or expired token is 404; a token for another e-mail is
// 403 (AUTHN-07). Failing rolls the whole sign-in back, so a wrong-account
// attempt leaves nothing behind.
func (s *Service) Accept(ctx context.Context, tx *db.Tx, userID uuid.UUID, email, token string) error {
	inv, found, err := inviteByHash(ctx, tx, crypt.HashToken(token))
	if err != nil {
		return err
	}
	if !found || !inv.Usable {
		return apperr.New(apperr.NotFound)
	}
	if inv.Email != strings.ToLower(email) {
		return apperr.New(apperr.Forbidden)
	}
	if err := grant(ctx, tx, userID, inv.ShopID, inv.Role, inv.CanEditProducts); err != nil {
		return err
	}
	return markAccepted(ctx, tx, inv.ID)
}

// Change edits a member's role or product-edit flag. Nobody edits themselves,
// and owners are not changed here (AUTHZ-04).
func (s *Service) Change(ctx context.Context, scope db.Scope, shopID, id uuid.UUID, in ChangeInput) error {
	v := validate.New().Check("body", in.Role != nil || in.CanEditProducts != nil, "required")
	if in.Role != nil {
		v.Check("role", consoleRoles(*in.Role), "invalid")
	}
	if err := v.Err(); err != nil {
		return err
	}
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		m, found, err := memberOf(ctx, tx, shopID, id)
		if err != nil {
			return err
		}
		if !found {
			return apperr.New(apperr.NotFound)
		}
		if m.UserID == scope.UserID || m.Role == enum.RoleShopOwner {
			return apperr.New(apperr.Forbidden)
		}
		role := m.Role
		if in.Role != nil {
			role = *in.Role
		}
		wasEdit, err := currentCanEdit(ctx, tx, id)
		if err != nil {
			return err
		}
		canEdit := wasEdit
		if in.CanEditProducts != nil {
			canEdit = *in.CanEditProducts
		}
		if role != enum.RoleShopStaff {
			canEdit = false
		}
		type state struct {
			Role            enum.Role `json:"role"`
			CanEditProducts bool      `json:"canEditProducts"`
		}
		if err := updateMember(ctx, tx, id, role, canEdit); err != nil {
			if db.IsUniqueViolation(err) {
				return apperr.New(apperr.Conflict)
			}
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "team.change", Entity: "membership", EntityID: id.String(), ShopID: shopID,
			Before: state{m.Role, wasEdit}, After: state{role, canEdit},
		})
	})
}

// Remove ends a membership. The last owner of a shop cannot be removed.
func (s *Service) Remove(ctx context.Context, scope db.Scope, shopID, id uuid.UUID) error {
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		m, found, err := memberOf(ctx, tx, shopID, id)
		if err != nil {
			return err
		}
		if !found {
			return apperr.New(apperr.NotFound)
		}
		if m.Role == enum.RoleShopOwner {
			n, err := otherOwners(ctx, tx, shopID, id)
			if err != nil {
				return err
			}
			if n == 0 {
				return apperr.New(apperr.Conflict)
			}
		}
		if err := removeMember(ctx, tx, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "team.remove", Entity: "membership", EntityID: id.String(), ShopID: shopID,
			Before: map[string]any{"role": m.Role, "userId": m.UserID},
		})
	})
}

func shopOf(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
