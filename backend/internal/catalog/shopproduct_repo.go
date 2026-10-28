package catalog

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/shared/db"
)

// shopProductSelect reads a shop product with the master product's facts.
// trim_scale makes a whole stock read "12", not "12.000".
const shopProductSelect = `
	SELECT sp.id, sp.product_id, p.name, p.brand, p.size_label, p.barcode, p.image_url,
	       sp.shop_price_cents, sp.mrp_cents, sp.online_price_cents, trim_scale(sp.stock_qty)::text,
	       sp.is_available, sp.max_per_order, sp.managed_by, sp.updated_at
	FROM shop_products sp JOIN products p ON p.id = sp.product_id `

func scanShopProduct(row pgx.Row) (ShopProduct, error) {
	var sp ShopProduct
	var stock *string
	err := row.Scan(&sp.ID, &sp.ProductID, &sp.Name, &sp.Brand, &sp.SizeLabel, &sp.Barcode, &sp.ImageURL,
		&sp.ShopPriceCents, &sp.MRPCents, &sp.OnlinePriceCents, &stock,
		&sp.IsAvailable, &sp.MaxPerOrder, &sp.ManagedBy, &sp.UpdatedAt)
	if stock != nil {
		n := json.Number(*stock)
		sp.StockQty = &n
	}
	return sp, err
}

// shopProductByID reads a product of THIS shop and locks it; one of another
// shop is not found.
func shopProductByID(ctx context.Context, tx *db.Tx, shopID, id uuid.UUID) (ShopProduct, bool, error) {
	sp, err := scanShopProduct(tx.QueryRow(ctx, shopProductSelect+`WHERE sp.id = $1 AND sp.shop_id = $2 FOR UPDATE OF sp`, id, shopID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ShopProduct{}, false, nil
	}
	return sp, err == nil, err
}

// listShopProducts returns one page plus one more row. available nil means all.
func listShopProducts(ctx context.Context, tx *db.Tx, shopID uuid.UUID, q string, available *bool, after *cursor) ([]ShopProduct, error) {
	var name *string
	var id uuid.UUID
	if after != nil {
		name, id = &after.Name, after.ID
	}
	rows, err := tx.Query(ctx, shopProductSelect+`
		WHERE sp.shop_id = $1
		  AND ($2 = '' OR p.search_doc LIKE lower($2))
		  AND ($3::boolean IS NULL OR sp.is_available = $3)
		  AND ($4::text IS NULL OR (p.name, sp.id) > ($4, $5))
		ORDER BY p.name, sp.id LIMIT $6`, shopID, likePattern(q), available, name, id, pageSize+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ShopProduct
	for rows.Next() {
		sp, err := scanShopProduct(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// saveShopProductFields updates only the fields the body carries. A stockQty of
// null clears the stock; one that is left out keeps it.
func saveShopProductFields(ctx context.Context, tx *db.Tx, id uuid.UUID, in ShopProductPatch) error {
	_, err := tx.Exec(ctx, `
		UPDATE shop_products SET
		  stock_qty     = CASE WHEN $2::boolean THEN $3::numeric ELSE stock_qty END,
		  is_available  = COALESCE($4, is_available),
		  max_per_order = COALESCE($5, max_per_order)
		WHERE id = $1`, id, in.StockQty.Set, in.StockQty.Ptr(), in.IsAvailable, in.MaxPerOrder)
	return err
}
