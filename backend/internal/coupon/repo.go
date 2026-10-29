package coupon

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
)

const columns = `id, code, shop_id, type, value, max_discount_cents, min_order_cents, starts_at, ends_at, usage_limit,
	per_user_limit, used_count, funded_by, allow_loss, status, created_at`

func scan(row pgx.Row) (Coupon, error) {
	var c Coupon
	err := row.Scan(&c.ID, &c.Code, &c.ShopID, &c.Type, &c.Value, &c.MaxDiscountCents, &c.MinOrderCents, &c.StartsAt, &c.EndsAt,
		&c.UsageLimit, &c.PerUserLimit, &c.UsedCount, &c.FundedBy, &c.AllowLoss, &c.Status, &c.CreatedAt)
	return c, err
}

func insert(ctx context.Context, tx *db.Tx, in CreateInput, createdBy uuid.UUID) (Coupon, error) {
	return scan(tx.QueryRow(ctx, `
		INSERT INTO coupons (code, shop_id, type, value, max_discount_cents, min_order_cents, starts_at, ends_at, usage_limit,
			per_user_limit, funded_by, allow_loss, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING `+columns,
		in.Code, in.ShopID, in.Type, in.Value, in.MaxDiscountCents, in.MinOrderCents, in.StartsAt, in.EndsAt, in.UsageLimit,
		in.PerUserLimit, in.FundedBy, in.AllowLoss, createdBy))
}

// lockByID reads a coupon and holds its row until the transaction ends.
func lockByID(ctx context.Context, tx *db.Tx, id uuid.UUID) (Coupon, bool, error) {
	c, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM coupons WHERE id = $1 FOR UPDATE`, id))
	return c, err == nil, db.IgnoreNoRows(err)
}

func byCode(ctx context.Context, tx *db.Tx, code string) (Coupon, bool, error) {
	c, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM coupons WHERE code = $1`, code))
	return c, err == nil, db.IgnoreNoRows(err)
}

func list(ctx context.Context, tx *db.Tx, status string, shopID *uuid.UUID) ([]Coupon, error) {
	rows, err := tx.Query(ctx, `
		SELECT `+columns+` FROM coupons
		WHERE ($1 = '' OR status = $1) AND ($2::uuid IS NULL OR shop_id = $2)
		ORDER BY created_at DESC, id DESC LIMIT $3`, status, shopID, listMax)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Coupon{}
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func save(ctx context.Context, tx *db.Tx, c Coupon) (Coupon, error) {
	return scan(tx.QueryRow(ctx, `
		UPDATE coupons SET status = $2, starts_at = $3, ends_at = $4, usage_limit = $5, per_user_limit = $6,
			max_discount_cents = $7, min_order_cents = $8, allow_loss = $9
		WHERE id = $1 RETURNING `+columns,
		c.ID, c.Status, c.StartsAt, c.EndsAt, c.UsageLimit, c.PerUserLimit, c.MaxDiscountCents, c.MinOrderCents, c.AllowLoss))
}

// reserve takes one use. The limit is checked in the same statement that counts
// the use, so two applications at once cannot both take the last one (BIZ-03).
// The row stays locked until the caller's transaction ends, which also keeps
// two applications of one coupon from counting the per-user uses at the same time.
func reserve(ctx context.Context, tx *db.Tx, id uuid.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE coupons SET used_count = used_count + 1
		WHERE id = $1 AND status = 'ACTIVE' AND (usage_limit IS NULL OR used_count < usage_limit)`, id)
	return tag.RowsAffected() == 1, err
}

// userUses counts the uses the customer holds of a coupon: reserved and confirmed.
func userUses(ctx context.Context, tx *db.Tx, couponID, userID uuid.UUID) (n int, err error) {
	err = tx.QueryRow(ctx, `SELECT count(*) FROM coupon_redemptions WHERE coupon_id = $1 AND user_id = $2`, couponID, userID).Scan(&n)
	return n, err
}

func insertRedemption(ctx context.Context, tx *db.Tx, couponID, orderID, userID uuid.UUID, amount int64) error {
	_, err := tx.Exec(ctx, `INSERT INTO coupon_redemptions (coupon_id, order_id, user_id, amount_cents) VALUES ($1, $2, $3, $4)`,
		couponID, orderID, userID, amount)
	return err
}

// release gives a reserved use back. A use that was confirmed by a payment is not touched.
func release(ctx context.Context, tx *db.Tx, orderID uuid.UUID) (bool, error) {
	tag, err := tx.Exec(ctx, `
		WITH gone AS (DELETE FROM coupon_redemptions WHERE order_id = $1 AND confirmed_at IS NULL RETURNING coupon_id)
		UPDATE coupons SET used_count = used_count - 1 WHERE id IN (SELECT coupon_id FROM gone)`, orderID)
	return tag.RowsAffected() == 1, err
}

func confirm(ctx context.Context, tx *db.Tx, orderID uuid.UUID, at time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE coupon_redemptions SET confirmed_at = $2 WHERE order_id = $1 AND confirmed_at IS NULL`, orderID, at)
	return err
}
