package user

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
)

// otpRow is the newest unused code of a user.
type otpRow struct {
	ID       uuid.UUID
	Phone    string
	Hash     []byte
	Attempts int
	Expires  time.Time
}

// replaceCode ends every earlier unused code of the user and stores the new one,
// so only the latest SMS works.
func replaceCode(ctx context.Context, tx *db.Tx, userID uuid.UUID, phone string, hash []byte, expires, now time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE otp_codes SET used_at = $2 WHERE user_id = $1 AND used_at IS NULL`, userID, now); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO otp_codes (user_id, phone, purpose, code_hash, expires_at)
		VALUES ($1, $2, 'PHONE_VERIFY', $3, $4)`, userID, phone, hash, expires)
	return err
}

// activeCode locks the newest unused code of the user, if there is one.
func activeCode(ctx context.Context, tx *db.Tx, userID uuid.UUID) (otpRow, bool, error) {
	var c otpRow
	err := tx.QueryRow(ctx, `
		SELECT id, phone, code_hash, attempts, expires_at FROM otp_codes
		WHERE user_id = $1 AND purpose = 'PHONE_VERIFY' AND used_at IS NULL
		ORDER BY created_at DESC, id DESC LIMIT 1 FOR UPDATE`, userID).
		Scan(&c.ID, &c.Phone, &c.Hash, &c.Attempts, &c.Expires)
	return c, err == nil, db.IgnoreNoRows(err)
}

func addAttempt(ctx context.Context, tx *db.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE otp_codes SET attempts = attempts + 1 WHERE id = $1`, id)
	return err
}

// confirmPhone marks the code used and the phone verified, in one step.
func confirmPhone(ctx context.Context, tx *db.Tx, userID, codeID uuid.UUID, phone string, now time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE otp_codes SET used_at = $2 WHERE id = $1`, codeID, now); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE users SET phone = $2, phone_verified_at = $3 WHERE id = $1`, userID, phone, now)
	return err
}
