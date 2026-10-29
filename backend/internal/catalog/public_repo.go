package catalog

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
)

// Every public product list reads the same columns, in the same order.
const cardSelect = `
	SELECT sp.id, sp.product_id, sp.shop_id, p.name, p.brand, p.size_label, p.unit, p.image_url,
	       sp.online_price_cents, sp.mrp_cents, sp.is_available, sp.max_per_order
	FROM shop_products sp JOIN products p ON p.id = sp.product_id `

// cardRow is a product card plus the shop it belongs to, which lists that mix
// shops turn into a slug.
type cardRow struct {
	ProductCard
	shopID uuid.UUID
}

func scanCards(rows pgx.Rows) ([]cardRow, error) {
	defer rows.Close()
	var out []cardRow
	for rows.Next() {
		var c cardRow
		if err := rows.Scan(&c.ShopProductID, &c.ProductID, &c.shopID, &c.Name, &c.Brand, &c.SizeLabel, &c.Unit, &c.ImageURL,
			&c.OnlinePriceCents, &c.MRPCents, &c.Available, &c.MaxPerOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// searchThreshold is how close a typo may be: pg_trgm word similarity from 0 to 1.
const searchThreshold = "0.4"

// useTypoSearch sets the similarity threshold for this transaction only. It is
// set_config(…, true), the parameterised form of SET LOCAL.
func useTypoSearch(ctx context.Context, tx *db.Tx) error {
	_, err := tx.Exec(ctx, `SELECT set_config('pg_trgm.word_similarity_threshold', $1, true)`, searchThreshold)
	return err
}

// shopCards is one page of a shop's products by name, optionally inside a
// category (and its children) and matching a search word.
func shopCards(ctx context.Context, tx *db.Tx, shopID uuid.UUID, category, q string, after *cursor, limit int) ([]cardRow, error) {
	var name *string
	var id uuid.UUID
	if after != nil {
		name, id = &after.Name, after.ID
	}
	rows, err := tx.Query(ctx, cardSelect+`
		WHERE sp.shop_id = $1 AND p.status = 'ACTIVE'
		  AND ($2 = '' OR p.category_id IN (SELECT c.id FROM categories c
		       WHERE c.slug = $2 OR c.parent_id = (SELECT id FROM categories WHERE slug = $2)))
		  AND ($3 = '' OR $3 <% p.search_doc)
		  AND ($4::text IS NULL OR (p.name, sp.id) > ($4, $5))
		ORDER BY p.name, sp.id LIMIT $6`, shopID, category, q, name, id, limit+1)
	if err != nil {
		return nil, err
	}
	return scanCards(rows)
}

// searchCards finds products of the given shops that match q, available ones
// first, cheapest first.
func searchCards(ctx context.Context, tx *db.Tx, shopIDs []uuid.UUID, q string, after *priceCursor, limit int) ([]cardRow, error) {
	ids := make([]string, len(shopIDs))
	for i, id := range shopIDs {
		ids[i] = id.String()
	}
	var unavailable bool
	var price int64
	var id uuid.UUID
	if after != nil {
		unavailable, price, id = after.Unavailable, after.Price, after.ID
	}
	rows, err := tx.Query(ctx, cardSelect+`
		WHERE sp.shop_id = ANY($1::uuid[]) AND p.status = 'ACTIVE' AND $2 <% p.search_doc
		  AND ($3::boolean IS NULL OR (NOT sp.is_available, sp.online_price_cents, sp.id) > ($3, $4, $5))
		ORDER BY NOT sp.is_available, sp.online_price_cents, sp.id LIMIT $6`,
		ids, q, nullIf(after != nil, unavailable), price, id, limit+1)
	if err != nil {
		return nil, err
	}
	return scanCards(rows)
}

// buyAgainCards is what the customer bought in this shop, latest first, at
// today's price. The scope of the transaction limits it to their own rows.
func buyAgainCards(ctx context.Context, tx *db.Tx, userID, shopID uuid.UUID, limit int) ([]cardRow, error) {
	rows, err := tx.Query(ctx, cardSelect+`
		JOIN customer_products cp ON cp.product_id = sp.product_id AND cp.shop_id = sp.shop_id
		WHERE cp.user_id = $1 AND cp.shop_id = $2 AND p.status = 'ACTIVE'
		ORDER BY cp.last_bought_at DESC, sp.id LIMIT $3`, userID, shopID, limit)
	if err != nil {
		return nil, err
	}
	return scanCards(rows)
}

type dealRow struct {
	Deal
	shopID uuid.UUID
}

// dealsOf are the available products with the biggest saving against the MRP.
func dealsOf(ctx context.Context, tx *db.Tx, shopIDs []uuid.UUID, limit int) ([]dealRow, error) {
	ids := make([]string, len(shopIDs))
	for i, id := range shopIDs {
		ids[i] = id.String()
	}
	rows, err := tx.Query(ctx, `
		SELECT sp.id, sp.shop_id, p.name, p.image_url, sp.online_price_cents, sp.mrp_cents,
		       sp.mrp_cents - sp.online_price_cents AS save
		FROM shop_products sp JOIN products p ON p.id = sp.product_id
		WHERE sp.shop_id = ANY($1::uuid[]) AND sp.is_available AND p.status = 'ACTIVE' AND sp.mrp_cents > sp.online_price_cents
		ORDER BY save DESC, sp.id LIMIT $2`, ids, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []dealRow
	for rows.Next() {
		var d dealRow
		if err := rows.Scan(&d.ShopProductID, &d.shopID, &d.Name, &d.ImageURL, &d.OnlinePriceCents, &d.MRPCents, &d.SaveCents); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// categoryCounts lists the categories with how many products the shop sells in
// each, parents included, and leaves out the empty ones.
func categoryCounts(ctx context.Context, tx *db.Tx, shopID uuid.UUID) ([]ShopCategory, error) {
	rows, err := tx.Query(ctx, `
		SELECT c.id, c.parent_id, c.slug, c.name, count(sp.id)
		FROM categories c
		LEFT JOIN products p ON p.category_id = c.id AND p.status = 'ACTIVE'
		LEFT JOIN shop_products sp ON sp.product_id = p.id AND sp.shop_id = $1
		GROUP BY c.id ORDER BY c.sort, c.name, c.id`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var all []ShopCategory
	for rows.Next() {
		var c ShopCategory
		if err := rows.Scan(&c.ID, &c.ParentID, &c.Slug, &c.Name, &c.Count); err != nil {
			return nil, err
		}
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rollUp(all), nil
}

// rollUp adds each child's count to its parent and drops categories with none.
func rollUp(all []ShopCategory) []ShopCategory {
	at := make(map[uuid.UUID]int, len(all))
	for i, c := range all {
		at[c.ID] = i
	}
	for _, c := range all {
		if c.ParentID != nil {
			if i, ok := at[*c.ParentID]; ok {
				all[i].Count += c.Count
			}
		}
	}
	out := make([]ShopCategory, 0, len(all))
	for _, c := range all {
		if c.Count > 0 {
			out = append(out, c)
		}
	}
	return out
}

// publicCategories is the whole tree, ordered by sort then name.
func publicCategories(ctx context.Context, tx *db.Tx) ([]CategoryNode, error) {
	rows, err := tx.Query(ctx, `SELECT id, parent_id, slug, name FROM categories ORDER BY sort, name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tops []CategoryNode
	kids := map[uuid.UUID][]CategoryNode{}
	for rows.Next() {
		var n CategoryNode
		var parent *uuid.UUID
		if err := rows.Scan(&n.ID, &parent, &n.Slug, &n.Name); err != nil {
			return nil, err
		}
		n.Children = []CategoryNode{}
		if parent == nil {
			tops = append(tops, n)
		} else {
			kids[*parent] = append(kids[*parent], n)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]CategoryNode, 0, len(tops))
	for _, t := range tops {
		if k := kids[t.ID]; k != nil {
			t.Children = k
		}
		out = append(out, t)
	}
	return out, nil
}

// nullIf gives a SQL NULL when there is no cursor, so the keyset test is skipped.
func nullIf(have bool, v bool) *bool {
	if !have {
		return nil
	}
	return &v
}
