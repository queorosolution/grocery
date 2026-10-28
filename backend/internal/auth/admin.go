package auth

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

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
func (s *Service) SetUserStatus(ctx context.Context, actor, id uuid.UUID, status enum.UserStatus) error {
	if err := validate.New().Check("status", status == enum.UserStatusActive || status == enum.UserStatusBlocked, "invalid").Err(); err != nil {
		return err
	}
	if id == actor && status != enum.UserStatusActive {
		return apperr.New(apperr.Forbidden)
	}
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET status = $2 WHERE id = $1 AND status <> 'DELETED'`, id, status)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.New(apperr.NotFound)
		}
		if status == enum.UserStatusBlocked {
			return revokeAllSessions(ctx, tx, id)
		}
		return nil
	})
}
