package pricing

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// chunk is how many rows one statement updates, to keep statements small.
const chunk = 1000

type priceRow struct {
	id         uuid.UUID
	categoryID *uuid.UUID
	shopPrice  money.Cents
	mrp        *money.Cents
	online     money.Cents
	markupBP   int
}

type currentRow struct {
	categoryID        *uuid.UUID
	shopPrice, online money.Cents
	mrp               *money.Cents
}

type priceChange struct {
	id                   uuid.UUID
	oldShop, newShop     money.Cents
	oldOnline, newOnline money.Cents
}

// lockRow reads one shop product of this shop and locks it for the update.
func lockRow(ctx context.Context, tx *db.Tx, shopID, id uuid.UUID) (cur currentRow, found bool, err error) {
	err = tx.QueryRow(ctx, `
		SELECT p.category_id, sp.shop_price_cents, sp.mrp_cents, sp.online_price_cents
		FROM shop_products sp JOIN products p ON p.id = sp.product_id
		WHERE sp.id = $1 AND sp.shop_id = $2
		FOR UPDATE OF sp`, id, shopID).Scan(&cur.categoryID, &cur.shopPrice, &cur.mrp, &cur.online)
	if errors.Is(err, pgx.ErrNoRows) {
		return cur, false, nil
	}
	return cur, err == nil, err
}

// lockShopRows reads every shop product of the shop with the markup that
// applies to it, and locks them.
func lockShopRows(ctx context.Context, tx *db.Tx, shopID uuid.UUID, defaultBP int) ([]priceRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT sp.id, p.category_id, sp.shop_price_cents, sp.mrp_cents, sp.online_price_cents,
		       COALESCE(cm.markup_bp, $2)
		FROM shop_products sp
		JOIN products p ON p.id = sp.product_id
		LEFT JOIN category_markups cm ON cm.shop_id = sp.shop_id AND cm.category_id = p.category_id
		WHERE sp.shop_id = $1
		ORDER BY sp.id
		FOR UPDATE OF sp`, shopID, defaultBP)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []priceRow
	for rows.Next() {
		var r priceRow
		if err := rows.Scan(&r.id, &r.categoryID, &r.shopPrice, &r.mrp, &r.online, &r.markupBP); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func categoryMarkup(ctx context.Context, tx *db.Tx, shopID, categoryID uuid.UUID) (bp int, found bool, err error) {
	err = tx.QueryRow(ctx, `SELECT markup_bp FROM category_markups WHERE shop_id = $1 AND category_id = $2`,
		shopID, categoryID).Scan(&bp)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return bp, err == nil, err
}

func storePrices(ctx context.Context, tx *db.Tx, id uuid.UUID, shop money.Cents, mrp *money.Cents, online money.Cents) error {
	_, err := tx.Exec(ctx, `
		UPDATE shop_products SET shop_price_cents = $2, mrp_cents = $3, online_price_cents = $4 WHERE id = $1`,
		id, shop, mrp, online)
	return err
}

// storeOnline updates only the customer price, in chunks.
func storeOnline(ctx context.Context, tx *db.Tx, changes []priceChange) error {
	for from := 0; from < len(changes); from += chunk {
		part := changes[from:min(from+chunk, len(changes))]
		ids, online := make([]string, len(part)), make([]int64, len(part))
		for i, c := range part {
			ids[i], online[i] = c.id.String(), int64(c.newOnline)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE shop_products sp SET online_price_cents = t.online
			FROM unnest($1::uuid[], $2::bigint[]) AS t(id, online)
			WHERE sp.id = t.id`, ids, online); err != nil {
			return err
		}
	}
	return nil
}

// logChanges appends the history rows. The actor is the signed-in user, or
// nobody for a system job.
func logChanges(ctx context.Context, tx *db.Tx, scope db.Scope, source enum.PriceChangeSource, changes []priceChange) error {
	by := actor(scope)
	for from := 0; from < len(changes); from += chunk {
		part := changes[from:min(from+chunk, len(changes))]
		n := len(part)
		ids, oldShop, newShop := make([]string, n), make([]int64, n), make([]int64, n)
		oldOnline, newOnline := make([]int64, n), make([]int64, n)
		for i, c := range part {
			ids[i], oldShop[i], newShop[i] = c.id.String(), int64(c.oldShop), int64(c.newShop)
			oldOnline[i], newOnline[i] = int64(c.oldOnline), int64(c.newOnline)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO price_changes (shop_product_id, old_shop_price, new_shop_price, old_online_price, new_online_price, source, changed_by)
			SELECT t.id, t.old_shop, t.new_shop, t.old_online, t.new_online, $6, $7
			FROM unnest($1::uuid[], $2::bigint[], $3::bigint[], $4::bigint[], $5::bigint[]) AS t(id, old_shop, new_shop, old_online, new_online)`,
			ids, oldShop, newShop, oldOnline, newOnline, source, by); err != nil {
			return err
		}
	}
	return nil
}

func markupsOf(ctx context.Context, tx *db.Tx, shopID uuid.UUID) ([]Markup, error) {
	rows, err := tx.Query(ctx, `SELECT category_id, markup_bp FROM category_markups WHERE shop_id = $1 ORDER BY category_id`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Markup{}
	for rows.Next() {
		var m Markup
		if err := rows.Scan(&m.CategoryID, &m.MarkupBP); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func replaceMarkups(ctx context.Context, tx *db.Tx, shopID uuid.UUID, in []Markup) error {
	if _, err := tx.Exec(ctx, `DELETE FROM category_markups WHERE shop_id = $1`, shopID); err != nil {
		return err
	}
	cats, bps := make([]string, len(in)), make([]int32, len(in))
	for i, m := range in {
		cats[i], bps[i] = m.CategoryID.String(), int32(m.MarkupBP) // #nosec G115 -- validated to 0..2000
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO category_markups (shop_id, category_id, markup_bp)
		SELECT $1, t.category_id, t.markup_bp FROM unnest($2::uuid[], $3::int[]) AS t(category_id, markup_bp)`,
		shopID, cats, bps)
	return err
}

// logFirstPrice writes the history row of a new shop product: no old prices.
func logFirstPrice(ctx context.Context, tx *db.Tx, scope db.Scope, source enum.PriceChangeSource, id uuid.UUID, shop, online money.Cents) error {
	by := actor(scope)
	_, err := tx.Exec(ctx, `
		INSERT INTO price_changes (shop_product_id, new_shop_price, new_online_price, source, changed_by)
		VALUES ($1, $2, $3, $4, $5)`, id, shop, online, source, by)
	return err
}

// actor is the signed-in user behind a change, or nil for a system job.
func actor(scope db.Scope) *uuid.UUID {
	if scope.UserID == uuid.Nil {
		return nil
	}
	return &scope.UserID
}
