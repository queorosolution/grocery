package connector

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/db"
)

// loadMatches looks up, in three queries, what the database already knows
// about a batch of rows. The result is keyed by external id. Both the preview
// and the run use it with Decide.
func loadMatches(ctx context.Context, tx *db.Tx, shopID uuid.UUID, items []Item) (map[string]Match, error) {
	extIDs := make([]string, 0, len(items))
	var barcodes []string
	for _, it := range items {
		extIDs = append(extIDs, it.ExternalID)
		if it.Barcode != nil {
			barcodes = append(barcodes, *it.Barcode)
		}
	}
	out := make(map[string]Match, len(items))

	rows, err := tx.Query(ctx, `
		SELECT external_id, id, shop_price_cents, mrp_cents, source_hash FROM shop_products
		WHERE shop_id = $1 AND external_id = ANY($2)`, shopID, extIDs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var ext string
		var e Existing
		if err := rows.Scan(&ext, &e.ShopProductID, &e.ShopPriceCents, &e.MRPCents, &e.SourceHash); err != nil {
			rows.Close()
			return nil, err
		}
		out[ext] = Match{Existing: &e}
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(barcodes) == 0 {
		return out, err
	}

	byBarcode := map[string]uuid.UUID{}
	rows, err = tx.Query(ctx, `SELECT barcode, id FROM products WHERE barcode = ANY($1)`, barcodes)
	if err != nil {
		return nil, err
	}
	var productIDs []string
	for rows.Next() {
		var code string
		var id uuid.UUID
		if err := rows.Scan(&code, &id); err != nil {
			rows.Close()
			return nil, err
		}
		byBarcode[code] = id
		productIDs = append(productIDs, id.String())
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(productIDs) == 0 {
		return out, err
	}

	linked := map[uuid.UUID]bool{}
	rows, err = tx.Query(ctx, `SELECT product_id FROM shop_products WHERE shop_id = $1 AND product_id = ANY($2::uuid[])`, shopID, productIDs)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		linked[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, it := range items {
		if it.Barcode == nil {
			continue
		}
		id, ok := byBarcode[*it.Barcode]
		if !ok {
			continue
		}
		m := out[it.ExternalID]
		m.ByBarcode, m.BarcodeLinked = &id, linked[id]
		out[it.ExternalID] = m
	}
	return out, nil
}
