package catalog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/cartclient"
	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/logx"
)

// Keeping the shop's cart price list current (docs/09 section 1). The price list
// shows prices in the cart; the bill never uses it.

// The cart service's limits for one item (docs/09 section 1). An item outside
// them is skipped and logged, so one bad row cannot block the others.
const (
	cartMaxName     = 120
	cartMaxPrice    = 999_999_999
	cartMaxImageURL = 500
)

// CartTenants finds a shop's cart tenant. The shop module implements it.
type CartTenants interface {
	CartTenant(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (tenantID string, ok bool, err error)
}

// PriceList is the part of the cart client the sync needs.
type PriceList interface {
	UpsertItems(ctx context.Context, tenantID string, items []cartclient.Item) error
}

// CartSync is the consumer of catalog.changed events.
type CartSync struct {
	tenants CartTenants
	list    PriceList
}

func NewCartSync(tenants CartTenants, list PriceList) *CartSync {
	return &CartSync{tenants: tenants, list: list}
}

// Register adds the consumer to the registry.
func (c *CartSync) Register(reg *events.Registry) {
	reg.On(enum.EventTypeCatalogChanged, events.Consumer{Name: "cart.prices", Handle: c.OnChanged})
}

// OnChanged re-reads the changed shop products and uploads them. The event
// carries ids only, so a late or repeated event still sends the current price.
// A failed upload is returned: the queue retries it and, after three tries,
// the DLQ alarm fires. Uploading an item again is harmless.
func (c *CartSync) OnChanged(ctx context.Context, tx *db.Tx, m events.Message) error {
	if m.ShopID == nil {
		return nil
	}
	var p struct {
		IDs []string `json:"shopProductIds"`
	}
	if err := json.Unmarshal(m.Data, &p); err != nil {
		return fmt.Errorf("cart.prices: bad event data: %w", err)
	}
	tenantID, ok, err := c.tenants.CartTenant(ctx, tx, *m.ShopID)
	if err != nil || !ok { // no cart tenant yet: the first upload happens when it is created
		return err
	}
	items, skipped, err := cartItems(ctx, tx, *m.ShopID, p.IDs)
	if err != nil {
		return err
	}
	if skipped > 0 {
		logx.From(ctx).Warn("cart.prices_skipped", "shop", m.ShopID.String(), "items", skipped)
	}
	if len(items) == 0 {
		return nil
	}
	return c.list.UpsertItems(ctx, tenantID, items)
}

// cartItems builds the items of this shop's products among ids. Ids of another
// shop or of a deleted row simply do not match.
func cartItems(ctx context.Context, tx *db.Tx, shopID uuid.UUID, ids []string) (items []cartclient.Item, skipped int, err error) {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, err := uuid.Parse(id); err == nil { // a malformed id cannot match; it must not fail the query
			valid = append(valid, id)
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT sp.id, p.name, sp.online_price_cents, sp.is_available AND p.status = 'ACTIVE', sp.max_per_order,
		       coalesce(p.image_url, '')
		FROM shop_products sp JOIN products p ON p.id = sp.product_id
		WHERE sp.shop_id = $1 AND sp.id = ANY($2::uuid[])
		ORDER BY sp.id`, shopID, valid)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var it cartclient.Item
		if err := rows.Scan(&it.ID, &it.Name, &it.UnitPrice, &it.Active, &it.MaxQuantity, &it.ImageURL); err != nil {
			return nil, 0, err
		}
		if validCartItem(it) {
			items = append(items, it)
		} else {
			skipped++
		}
	}
	return items, skipped, rows.Err()
}

func validCartItem(it cartclient.Item) bool {
	return len(it.Name) <= cartMaxName && it.UnitPrice >= 1 && it.UnitPrice <= cartMaxPrice &&
		it.MaxQuantity >= 1 && it.MaxQuantity <= 999 && len(it.ImageURL) <= cartMaxImageURL
}

// QueueFullSync announces every product of the shop as changed, so the whole
// price list is uploaded. It is used when the cart tenant is created and for a
// manual re-sync. It only writes events (outbox); no network call happens here.
func (s *Service) QueueFullSync(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (int, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM shop_products WHERE shop_id = $1 ORDER BY id`, shopID)
	if err != nil {
		return 0, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return len(ids), events.AddCatalogChanged(ctx, tx, shopID, ids)
}
