package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// Blocker is a module that knows about work still open for a person: an order
// not finished, a refund not paid. It answers with a field and a short reason;
// an empty answer means nothing is open.
type Blocker interface {
	OpenWork(ctx context.Context, tx *db.Tx, userID uuid.UUID) (map[string]string, error)
}

// Wiper is a module that holds personal details no record needs (a phone
// number, saved addresses). It removes them when the account is closed.
type Wiper interface {
	Wipe(ctx context.Context, tx *db.Tx, userID uuid.UUID) error
}

// OnDelete says which modules are asked before an account is closed and which
// clear their details afterwards. It is a setter because those modules are
// built after this one.
func (s *Service) OnDelete(blockers []Blocker, wipers []Wiper) {
	s.blockers, s.wipers = blockers, wipers
}

// DeleteAccount closes the caller's account (FR-AUTH-09). It is a soft delete:
// the row, the name and the e-mail stay so old orders and refunds keep their
// owner, but the person is signed out everywhere, cannot sign in again, and the
// phone and saved addresses are removed. It refuses while any module reports
// open work, with every reason at once so the page can list them.
func (s *Service) DeleteAccount(ctx context.Context, scope db.Scope, confirm string) error {
	if confirm != "DELETE" {
		return apperr.Validation(map[string]string{"confirm": "invalid"})
	}
	return s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		// The row lock also holds back a bill that is being sent at this moment.
		var status enum.UserStatus
		err := tx.QueryRow(ctx, `SELECT status FROM users WHERE id = $1 FOR UPDATE`, scope.UserID).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != enum.UserStatusActive) {
			return apperr.New(apperr.Unauthenticated)
		}
		if err != nil {
			return err
		}
		open := map[string]string{}
		for _, b := range s.blockers {
			part, err := b.OpenWork(ctx, tx, scope.UserID)
			if err != nil {
				return err
			}
			for k, v := range part {
				open[k] = v
			}
		}
		if len(open) > 0 {
			e := apperr.New(apperr.AccountNotDeletable)
			e.Fields = open
			return e
		}
		for _, w := range s.wipers {
			if err := w.Wipe(ctx, tx, scope.UserID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET status = 'DELETED' WHERE id = $1`, scope.UserID); err != nil {
			return err
		}
		if err := revokeAllSessions(ctx, tx, scope.UserID); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{
			Action: "user.delete", Entity: "user", EntityID: scope.UserID.String(),
			Before: map[string]enum.UserStatus{"status": status}, After: map[string]enum.UserStatus{"status": enum.UserStatusDeleted},
		})
	})
}
