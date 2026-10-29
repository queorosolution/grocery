package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// maxUserList caps a people search; admins narrow it with q.
const maxUserList = 50

// AdminUser is the admin's view of an account.
type AdminUser struct {
	ID          uuid.UUID       `json:"id"`
	Name        string          `json:"name"`
	Email       string          `json:"email"`
	PhoneMasked *string         `json:"phoneMasked"`
	Status      enum.UserStatus `json:"status"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// likePrefix turns user text into a safe LIKE prefix pattern.
func likePrefix(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(strings.ToLower(strings.TrimSpace(q))) + "%"
}

// SearchUsers lists accounts whose e-mail or name starts with q, newest first.
func (s *Service) SearchUsers(ctx context.Context, q string) ([]AdminUser, error) {
	out := []AdminUser{}
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, name, email, phone, status, created_at FROM users
			WHERE $1 = '%' OR email LIKE $1 OR lower(name) LIKE $1
			ORDER BY created_at DESC, id LIMIT $2`, likePrefix(q), maxUserList)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var u AdminUser
			var phone *string
			if err := rows.Scan(&u.ID, &u.Name, &u.Email, &phone, &u.Status, &u.CreatedAt); err != nil {
				return err
			}
			if phone != nil {
				m := maskPhone(*phone)
				u.PhoneMasked = &m
			}
			out = append(out, u)
		}
		return rows.Err()
	})
	return out, err
}

// SetUserStatus blocks or unblocks an account. Blocking ends every session at
// once (AUTHN-08). An admin cannot block themselves.
func (s *Service) SetUserStatus(ctx context.Context, scope db.Scope, id uuid.UUID, status enum.UserStatus) error {
	if err := validate.New().Check("status", status == enum.UserStatusActive || status == enum.UserStatusBlocked, "invalid").Err(); err != nil {
		return err
	}
	if id == scope.UserID && status != enum.UserStatusActive {
		return apperr.New(apperr.Forbidden)
	}
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		var before enum.UserStatus
		err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1 AND status <> 'DELETED' FOR UPDATE`, id).Scan(&before)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.NotFound)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET status = $2 WHERE id = $1`, id, status); err != nil {
			return err
		}
		if status == enum.UserStatusBlocked {
			if err := revokeAllSessions(ctx, tx, id); err != nil {
				return err
			}
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "user.status", Entity: "user", EntityID: id.String(),
			Before: map[string]enum.UserStatus{"status": before}, After: map[string]enum.UserStatus{"status": status},
		})
	})
}

// RestoreUser brings a deleted account back (soft delete keeps the row). The name and e-mail were
// never removed, so the person signs in as before. What the delete wiped (see Wiper) does not come back.
func (s *Service) RestoreUser(ctx context.Context, scope db.Scope, id uuid.UUID) error {
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		var found uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 AND status = 'DELETED' FOR UPDATE`, id).Scan(&found)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.NotFound)
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET status = 'ACTIVE' WHERE id = $1`, id); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "user.restore", Entity: "user", EntityID: id.String(),
			Before: map[string]enum.UserStatus{"status": enum.UserStatusDeleted}, After: map[string]enum.UserStatus{"status": enum.UserStatusActive},
		})
	})
}
