package stats

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
)

// add counts d into the row of the shop and the Colombo day of at. One statement
// creates the row or adds to it, so two events of the same day never lose a count.
func add(ctx context.Context, tx *db.Tx, shopID uuid.UUID, at time.Time, d Sums) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO shop_daily_stats AS t (shop_id, day, orders_submitted, orders_paid, orders_delivered, orders_rejected, orders_expired,
			orders_cancelled, gmv_cents, shop_items_cents, platform_income_cents, discount_cents,
			review_seconds_sum, review_count, pick_seconds_sum, pick_count)
		VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (shop_id, day) DO UPDATE SET
			orders_submitted = t.orders_submitted + EXCLUDED.orders_submitted, orders_paid = t.orders_paid + EXCLUDED.orders_paid,
			orders_delivered = t.orders_delivered + EXCLUDED.orders_delivered, orders_rejected = t.orders_rejected + EXCLUDED.orders_rejected,
			orders_expired = t.orders_expired + EXCLUDED.orders_expired, orders_cancelled = t.orders_cancelled + EXCLUDED.orders_cancelled,
			gmv_cents = t.gmv_cents + EXCLUDED.gmv_cents, shop_items_cents = t.shop_items_cents + EXCLUDED.shop_items_cents,
			platform_income_cents = t.platform_income_cents + EXCLUDED.platform_income_cents, discount_cents = t.discount_cents + EXCLUDED.discount_cents,
			review_seconds_sum = t.review_seconds_sum + EXCLUDED.review_seconds_sum, review_count = t.review_count + EXCLUDED.review_count,
			pick_seconds_sum = t.pick_seconds_sum + EXCLUDED.pick_seconds_sum, pick_count = t.pick_count + EXCLUDED.pick_count`,
		shopID, clock.Colombo(at).Format(time.DateOnly), d.Submitted, d.Paid, d.Delivered, d.Rejected, d.Expired, d.Cancelled,
		d.GMVCents, d.ShopItemsCents, d.PlatformIncomeCents, d.DiscountCents, d.ReviewSeconds, d.ReviewCount, d.PickSeconds, d.PickCount)
	return err
}

// totalOf adds the rows of the range. A nil shop means every shop.
func totalOf(ctx context.Context, tx *db.Tx, shopID *uuid.UUID, r Range) (s Sums, err error) {
	err = tx.QueryRow(ctx, `SELECT `+sumCols+` FROM shop_daily_stats WHERE ($1::uuid IS NULL OR shop_id = $1) AND day BETWEEN $2::date AND $3::date`,
		shopID, r.fromText(), r.toText()).Scan(s.fields()...)
	return s, err
}

// daysOf is the sales per day over the range, for the chart. Days with no row are
// left out; the screen draws them as empty.
func daysOf(ctx context.Context, tx *db.Tx, shopID *uuid.UUID, r Range) ([]Day, error) {
	rows, err := tx.Query(ctx, `
		SELECT day::text, COALESCE(sum(gmv_cents), 0)::bigint, COALESCE(sum(orders_paid), 0)::int FROM shop_daily_stats
		WHERE ($1::uuid IS NULL OR shop_id = $1) AND day BETWEEN $2::date AND $3::date GROUP BY day ORDER BY day`,
		shopID, r.fromText(), r.toText())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Day{}
	for rows.Next() {
		var d Day
		if err := rows.Scan(&d.Day, &d.SalesCents, &d.Orders); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
