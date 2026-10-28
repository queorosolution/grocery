package pricing

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/events"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// NewShopProduct is a product being put on sale in a shop for the first time.
type NewShopProduct struct {
	ShopID, ProductID uuid.UUID
	ExternalID        *string
	ShopPrice         money.Cents
	MRP               *money.Cents
	StockQty          *int64
	Available         bool
	MaxPerOrder       int
	ManagedBy         enum.ManagedBy
	Source            enum.PriceChangeSource
}

// AddShopProduct inserts the shop_products row with its first customer price,
// the first price_changes row (no old price) and a catalog.changed event, all
// in the caller's transaction. A unique clash (the shop already sells the
// product) and a missing product come back as database errors for the caller to
// name; ErrMRPBelowShopPrice as is.
func (s *Service) AddShopProduct(ctx context.Context, tx *db.Tx, scope db.Scope, n NewShopProduct) (uuid.UUID, Applied, error) {
	var category *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT category_id FROM products WHERE id = $1`, n.ProductID).Scan(&category)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, Applied{}, apperr.Validation(map[string]string{"productId": "unknown"})
	}
	if err != nil {
		return uuid.Nil, Applied{}, err
	}
	bp, err := s.markupFor(ctx, tx, n.ShopID, category)
	if err != nil {
		return uuid.Nil, Applied{}, err
	}
	online, err := OnlinePrice(n.ShopPrice, n.MRP, bp)
	if err != nil {
		return uuid.Nil, Applied{}, err
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO shop_products (shop_id, product_id, external_id, mrp_cents, shop_price_cents, online_price_cents,
		                          stock_qty, is_available, max_per_order, managed_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`,
		n.ShopID, n.ProductID, n.ExternalID, n.MRP, n.ShopPrice, online, n.StockQty, n.Available, n.MaxPerOrder, n.ManagedBy).Scan(&id)
	if err != nil {
		return uuid.Nil, Applied{}, err
	}
	if err := logFirstPrice(ctx, tx, scope, n.Source, id, n.ShopPrice, online); err != nil {
		return uuid.Nil, Applied{}, err
	}
	err = events.AddCatalogChanged(ctx, tx, n.ShopID, []uuid.UUID{id})
	return id, Applied{ShopPrice: n.ShopPrice, Online: online, MRP: n.MRP, Changed: true}, err
}
