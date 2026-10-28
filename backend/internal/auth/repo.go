package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// All auth queries run in the system scope: sessions and admin_totp are
// privileged-only tables, and sign-in happens before there is a user to scope to.

type userRow struct {
	ID            uuid.UUID
	Email         string
	Name          string
	Phone         *string
	PhoneVerified bool
	Status        enum.UserStatus
}

const userColumns = `id, email, name, phone, phone_verified_at IS NOT NULL, status`

func scanUser(row pgx.Row) (userRow, error) {
	var u userRow
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.Phone, &u.PhoneVerified, &u.Status)
	return u, err
}

func userByID(ctx context.Context, tx *db.Tx, id uuid.UUID) (userRow, bool, error) {
	u, err := scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id))
	return u, err == nil, notFoundOK(err)
}

// upsertUser creates the user on first sign-in and never changes an existing
// one: the name may have been edited in the account page since.
func upsertUser(ctx context.Context, tx *db.Tx, g GoogleIdentity, email, name string) (userRow, error) {
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (google_sub, email, name) VALUES ($1, $2, $3)
		ON CONFLICT (google_sub) DO NOTHING`, g.Sub, email, name); err != nil {
		return userRow{}, err
	}
	return scanUser(tx.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE google_sub = $1`, g.Sub))
}

func ensureSuperAdmin(ctx context.Context, tx *db.Tx, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO memberships (user_id, scope, role) VALUES ($1, 'PLATFORM', 'SUPER_ADMIN')
		ON CONFLICT DO NOTHING`, userID)
	return err
}

type sessionRow struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	TotpOK    bool
	ExpiresAt time.Time
	Rotated   bool
	Revoked   bool
}

type newSession struct {
	UserID    uuid.UUID
	FamilyID  uuid.UUID
	Hash      []byte
	TotpOK    bool
	ExpiresAt time.Time
	UserAgent string
	IPHash    []byte
}

func insertSession(ctx context.Context, tx *db.Tx, s newSession) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO sessions (user_id, family_id, refresh_hash, totp_ok, expires_at, user_agent, ip_hash)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)
		RETURNING id`,
		s.UserID, s.FamilyID, s.Hash, s.TotpOK, s.ExpiresAt, s.UserAgent, s.IPHash).Scan(&id)
	return id, err
}

// sessionByRefreshHash locks the row, so two refreshes with the same token
// cannot both succeed.
func sessionByRefreshHash(ctx context.Context, tx *db.Tx, hash []byte) (sessionRow, bool, error) {
	var s sessionRow
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, family_id, totp_ok, expires_at, rotated_at IS NOT NULL, revoked_at IS NOT NULL
		FROM sessions WHERE refresh_hash = $1 FOR UPDATE`, hash).
		Scan(&s.ID, &s.UserID, &s.FamilyID, &s.TotpOK, &s.ExpiresAt, &s.Rotated, &s.Revoked)
	return s, err == nil, notFoundOK(err)
}

func markRotated(ctx context.Context, tx *db.Tx, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE sessions SET rotated_at = now() WHERE id = $1`, id)
	return err
}

func revokeFamily(ctx context.Context, tx *db.Tx, familyID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE family_id = $1 AND revoked_at IS NULL`, familyID)
	return err
}

// revokeSessionFamily ends the sign-in a session belongs to. Scoping by user
// id means a forged session id can never touch someone else's rows.
func revokeSessionFamily(ctx context.Context, tx *db.Tx, userID, sessionID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE revoked_at IS NULL AND user_id = $1
		  AND family_id = (SELECT family_id FROM sessions WHERE id = $2 AND user_id = $1)`, userID, sessionID)
	return err
}

func revokeAllSessions(ctx context.Context, tx *db.Tx, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

// totpEnabled reports whether the user finished authenticator setup.
func totpEnabled(ctx context.Context, tx *db.Tx, userID uuid.UUID) (bool, error) {
	var enabled bool
	err := tx.QueryRow(ctx, `SELECT enabled_at IS NOT NULL FROM admin_totp WHERE user_id = $1`, userID).Scan(&enabled)
	return enabled, notFoundOK(err)
}

// notFoundOK turns "no rows" into a plain false for the caller.
func notFoundOK(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

// totpRow is the stored authenticator secret of a super admin.
type totpRow struct {
	SecretEnc    []byte
	Enabled      bool
	LastUsedStep int64
}

// totpForUpdate locks the row, so two codes cannot both pass the replay check.
func totpForUpdate(ctx context.Context, tx *db.Tx, userID uuid.UUID) (totpRow, bool, error) {
	var t totpRow
	err := tx.QueryRow(ctx, `
		SELECT secret_enc, enabled_at IS NOT NULL, last_used_step FROM admin_totp WHERE user_id = $1 FOR UPDATE`, userID).
		Scan(&t.SecretEnc, &t.Enabled, &t.LastUsedStep)
	return t, err == nil, notFoundOK(err)
}

// saveTotpSecret stores a pending secret. A finished setup is never replaced.
func saveTotpSecret(ctx context.Context, tx *db.Tx, userID uuid.UUID, sealed []byte) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO admin_totp (user_id, secret_enc) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET secret_enc = EXCLUDED.secret_enc, last_used_step = 0
		WHERE admin_totp.enabled_at IS NULL`, userID, sealed)
	return tag.RowsAffected() == 1, err
}

// useTotpStep records the accepted step and turns TOTP on at the first success.
func useTotpStep(ctx context.Context, tx *db.Tx, userID uuid.UUID, step int64) error {
	_, err := tx.Exec(ctx, `
		UPDATE admin_totp SET last_used_step = $2, enabled_at = coalesce(enabled_at, now()) WHERE user_id = $1`, userID, step)
	return err
}

func markSessionTotp(ctx context.Context, tx *db.Tx, userID, sessionID uuid.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE sessions SET totp_ok = true WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, sessionID, userID)
	return tag.RowsAffected() == 1, err
}
