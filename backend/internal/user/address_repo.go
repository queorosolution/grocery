package user

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
)

const addressColumns = `id, label, line1, landmark, lat, lng, is_default`

func scanAddress(row pgx.Row) (Address, error) {
	var a Address
	err := row.Scan(&a.ID, &a.Label, &a.Line1, &a.Landmark, &a.Lat, &a.Lng, &a.IsDefault)
	return a, err
}

func listAddresses(ctx context.Context, tx *db.Tx, userID uuid.UUID) ([]Address, error) {
	rows, err := tx.Query(ctx, `SELECT `+addressColumns+` FROM addresses WHERE user_id = $1
		ORDER BY is_default DESC, created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Address{}
	for rows.Next() {
		a, err := scanAddress(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// lockUser serialises one user's address changes, so two requests at once
// cannot both pass the "max 5" check or leave two defaults.
func lockUser(ctx context.Context, tx *db.Tx, userID uuid.UUID) error {
	var id uuid.UUID
	return tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&id)
}

func countAddresses(ctx context.Context, tx *db.Tx, userID uuid.UUID) (int, error) {
	var n int
	err := tx.QueryRow(ctx, `SELECT count(*) FROM addresses WHERE user_id = $1`, userID).Scan(&n)
	return n, err
}

// clearDefault must run before a new default is written: the unique index
// allows one default per user.
func clearDefault(ctx context.Context, tx *db.Tx, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `UPDATE addresses SET is_default = false WHERE user_id = $1 AND is_default`, userID)
	return err
}

func insertAddress(ctx context.Context, tx *db.Tx, userID uuid.UUID, in AddressInput, isDefault bool) (Address, error) {
	return scanAddress(tx.QueryRow(ctx, `
		INSERT INTO addresses (user_id, label, line1, landmark, lat, lng, is_default)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7) RETURNING `+addressColumns,
		userID, in.Label, in.Line1, deref(in.Landmark), in.Lat, in.Lng, isDefault))
}

// addressByID reads and locks the row; another user's id is simply not found.
func addressByID(ctx context.Context, tx *db.Tx, userID, id uuid.UUID) (Address, bool, error) {
	a, err := scanAddress(tx.QueryRow(ctx, `SELECT `+addressColumns+` FROM addresses
		WHERE id = $1 AND user_id = $2 FOR UPDATE`, id, userID))
	return a, err == nil, db.IgnoreNoRows(err)
}

// addressOf is addressByID without the lock, for callers that only read.
func addressOf(ctx context.Context, tx *db.Tx, userID, id uuid.UUID) (Address, bool, error) {
	a, err := scanAddress(tx.QueryRow(ctx, `SELECT `+addressColumns+` FROM addresses WHERE id = $1 AND user_id = $2`, id, userID))
	return a, err == nil, db.IgnoreNoRows(err)
}

func saveAddress(ctx context.Context, tx *db.Tx, userID uuid.UUID, a Address) (Address, error) {
	return scanAddress(tx.QueryRow(ctx, `
		UPDATE addresses SET label = $3, line1 = $4, landmark = NULLIF($5, ''), lat = $6, lng = $7, is_default = $8
		WHERE id = $1 AND user_id = $2 RETURNING `+addressColumns,
		a.ID, userID, a.Label, a.Line1, deref(a.Landmark), a.Lat, a.Lng, a.IsDefault))
}

func deleteAddress(ctx context.Context, tx *db.Tx, userID, id uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM addresses WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// promoteNewest makes the newest address the default when none is left.
func promoteNewest(ctx context.Context, tx *db.Tx, userID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE addresses SET is_default = true
		WHERE id = (SELECT id FROM addresses WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT 1)
		  AND NOT EXISTS (SELECT 1 FROM addresses WHERE user_id = $1 AND is_default)`, userID)
	return err
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
