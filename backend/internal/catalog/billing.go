package catalog

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/shared/db"
	"gp/internal/shared/money"
)

// noCategoryWeightG is the pack weight used when neither the product nor a
// category says. It matches the column default of categories.default_weight_g.
const noCategoryWeightG = 500

// BillProducts reads the shop products of one shop that the bill builder asks
// for, as the database has them now. A product is available only when the shop
// sells it and the master product is not hidden. Ids of another shop, and
// unknown ids, are left out of the map; Build treats them as not for sale.
func (s *Service) BillProducts(ctx context.Context, tx *db.Tx, shopID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]billing.ProductSnapshot, error) {
	out := make(map[uuid.UUID]billing.ProductSnapshot, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	keys := make([]string, len(ids)) // pgx cannot encode []uuid.UUID for uuid[]
	for i, id := range ids {
		keys[i] = id.String()
	}
	rows, err := tx.Query(ctx, `
		SELECT sp.id, p.name, p.unit, sp.shop_price_cents, sp.online_price_cents,
		       sp.is_available AND p.status = 'ACTIVE', sp.max_per_order,
		       COALESCE(p.weight_g, c.default_weight_g, $3)
		FROM shop_products sp
		JOIN products p ON p.id = sp.product_id
		LEFT JOIN categories c ON c.id = p.category_id
		WHERE sp.shop_id = $1 AND sp.id = ANY($2::uuid[])`, shopID, keys, noCategoryWeightG)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p billing.ProductSnapshot
		var shopPrice, online int64
		if err := rows.Scan(&p.ID, &p.Name, &p.Unit, &shopPrice, &online, &p.Available, &p.MaxPerOrder, &p.WeightG); err != nil {
			return nil, err
		}
		p.ShopPriceCents, p.OnlinePriceCents = money.Cents(shopPrice), money.Cents(online)
		out[p.ID] = p
	}
	return out, rows.Err()
}
