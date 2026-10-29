package user

import (
	"context"
	"strings"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/validate"
)

const maxNameRunes = 80

// SetName changes the display name. The e-mail comes from Google and cannot be changed here.
func (s *Service) SetName(ctx context.Context, scope db.Scope, name string) error {
	name = strings.TrimSpace(name)
	if err := validate.New().Str("name", name, 1, maxNameRunes).Err(); err != nil {
		return err
	}
	return s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE users SET name = $2 WHERE id = $1`, scope.UserID, name)
		if err == nil && tag.RowsAffected() != 1 {
			err = apperr.New(apperr.Unauthenticated)
		}
		return err
	})
}
