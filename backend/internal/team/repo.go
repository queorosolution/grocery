package team

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"gp/internal/shared/authz"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

func membershipsOf(ctx context.Context, tx *db.Tx, userID uuid.UUID) ([]authz.Membership, error) {
	rows, err := tx.Query(ctx, `
		SELECT m.id, m.role, m.shop_id, s.slug, s.name, m.can_edit_products
		FROM memberships m LEFT JOIN shops s ON s.id = m.shop_id
		WHERE m.user_id = $1 AND m.status = 'ACTIVE'
		ORDER BY m.created_at, m.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []authz.Membership
	for rows.Next() {
		var m authz.Membership
		var shopID *uuid.UUID
		var slug, name *string
		if err := rows.Scan(&m.ID, &m.Role, &shopID, &slug, &name, &m.CanEditProducts); err != nil {
			return nil, err
		}
		if shopID != nil {
			m.ShopID, m.ShopSlug, m.ShopName = *shopID, deref(slug), deref(name)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func membersOf(ctx context.Context, tx *db.Tx, shopID uuid.UUID) ([]Member, error) {
	rows, err := tx.Query(ctx, `
		SELECT m.id, u.id, u.name, u.email, m.role, m.can_edit_products
		FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.shop_id = $1 AND m.status = 'ACTIVE'
		ORDER BY m.created_at, m.id`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Member{}
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.ID, &m.UserID, &m.Name, &m.Email, &m.Role, &m.CanEditProducts); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func pendingInvites(ctx context.Context, tx *db.Tx, shopID uuid.UUID) ([]Invite, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, email, shop_id, role, can_edit_products, expires_at
		FROM invites
		WHERE shop_id = $1 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > $2
		ORDER BY created_at, id`, shopID, clock.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invite{}
	for rows.Next() {
		var i Invite
		if err := rows.Scan(&i.ID, &i.Email, &i.ShopID, &i.Role, &i.CanEditProducts, &i.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func shopExists(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM shops WHERE id = $1)`, shopID).Scan(&ok)
	return ok, err
}

type newInvite struct {
	Email           string
	ShopID          *uuid.UUID
	Role            enum.Role
	CanEditProducts bool
	TokenHash       []byte
	InvitedBy       uuid.UUID
	ExpiresAt       time.Time
}

// insertInvite replaces any pending invite for the same e-mail and shop, so a
// shop never has two live links for one person.
func insertInvite(ctx context.Context, tx *db.Tx, n newInvite) (Invite, error) {
	if _, err := tx.Exec(ctx, `
		UPDATE invites SET revoked_at = now()
		WHERE email = $1 AND shop_id IS NOT DISTINCT FROM $2 AND accepted_at IS NULL AND revoked_at IS NULL`,
		n.Email, n.ShopID); err != nil {
		return Invite{}, err
	}
	i := Invite{Email: n.Email, ShopID: n.ShopID, Role: n.Role, CanEditProducts: n.CanEditProducts, ExpiresAt: n.ExpiresAt}
	err := tx.QueryRow(ctx, `
		INSERT INTO invites (email, shop_id, role, can_edit_products, token_hash, invited_by, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		n.Email, n.ShopID, n.Role, n.CanEditProducts, n.TokenHash, n.InvitedBy, n.ExpiresAt).Scan(&i.ID)
	return i, err
}

func revokeInvite(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE invites SET revoked_at = now() WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL`, id)
	return tag.RowsAffected() == 1, err
}

type inviteRow struct {
	ID              uuid.UUID
	Email           string
	ShopID          *uuid.UUID
	Role            enum.Role
	CanEditProducts bool
	Usable          bool // not accepted, not revoked, not expired
}

// inviteByHash locks the row, so two sign-ins cannot both accept one invite.
func inviteByHash(ctx context.Context, tx *db.Tx, hash []byte) (inviteRow, bool, error) {
	var r inviteRow
	err := tx.QueryRow(ctx, `
		SELECT id, email, shop_id, role, can_edit_products,
		       accepted_at IS NULL AND revoked_at IS NULL AND expires_at > $2
		FROM invites WHERE token_hash = $1 FOR UPDATE`, hash, clock.Now()).
		Scan(&r.ID, &r.Email, &r.ShopID, &r.Role, &r.CanEditProducts, &r.Usable)
	if errors.Is(err, pgx.ErrNoRows) {
		return inviteRow{}, false, nil
	}
	return r, err == nil, err
}

func markAccepted(ctx context.Context, tx *db.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE invites SET accepted_at = now() WHERE id = $1`, id)
	return err
}

// grant adds the membership, or turns a removed one back on.
func grant(ctx context.Context, tx *db.Tx, userID uuid.UUID, shopID *uuid.UUID, role enum.Role, canEdit bool) error {
	scope := enum.MembershipScopePlatform
	if shopID != nil {
		scope = enum.MembershipScopeShop
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO memberships (user_id, scope, shop_id, role, can_edit_products)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, (coalesce(shop_id, '00000000-0000-0000-0000-000000000000'::uuid)), role)
		DO UPDATE SET status = 'ACTIVE', can_edit_products = EXCLUDED.can_edit_products`,
		userID, scope, shopID, role, canEdit)
	return err
}

type memberRow struct {
	ID     uuid.UUID
	UserID uuid.UUID
	Role   enum.Role
}

// memberOf finds an active membership in this shop and locks it.
func memberOf(ctx context.Context, tx *db.Tx, shopID, id uuid.UUID) (memberRow, bool, error) {
	var m memberRow
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, role FROM memberships
		WHERE id = $1 AND shop_id = $2 AND status = 'ACTIVE' FOR UPDATE`, id, shopID).
		Scan(&m.ID, &m.UserID, &m.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return memberRow{}, false, nil
	}
	return m, err == nil, err
}

func updateMember(ctx context.Context, tx *db.Tx, id uuid.UUID, role enum.Role, canEdit bool) error {
	_, err := tx.Exec(ctx, `UPDATE memberships SET role = $2, can_edit_products = $3 WHERE id = $1`, id, role, canEdit)
	return err
}

func currentCanEdit(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error) {
	var v bool
	err := tx.QueryRow(ctx, `SELECT can_edit_products FROM memberships WHERE id = $1`, id).Scan(&v)
	return v, err
}

func removeMember(ctx context.Context, tx *db.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE memberships SET status = 'REMOVED' WHERE id = $1`, id)
	return err
}

// otherOwners counts active owners of the shop besides the given membership,
// locking them so two removals at once cannot both pass.
func otherOwners(ctx context.Context, tx *db.Tx, shopID, except uuid.UUID) (int, error) {
	rows, err := tx.Query(ctx, `
		SELECT id FROM memberships
		WHERE shop_id = $1 AND role = 'SHOP_OWNER' AND status = 'ACTIVE' AND id <> $2 FOR UPDATE`, shopID, except)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	return n, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
