package catalog

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// BuyAgain keeps customer_products, the list behind "buy again" (FR-CAT-07): what
// each customer got delivered, per shop.
type BuyAgain struct{}

func NewBuyAgain() *BuyAgain { return &BuyAgain{} }

// Register adds the consumer to the registry.
func (b *BuyAgain) Register(reg *events.Registry) {
	reg.On(enum.EventTypeOrderDelivered, events.Consumer{Name: "catalog.buy_again", Handle: b.OnDelivered})
}

// OnDelivered counts the lines the customer received: the lines of the order's
// current revision that were kept or substituted (a removed or missing line was
// not bought). One row per product, so a repeated line counts once. The event
// carries only the order id; the dispatcher claims each (event, consumer) once,
// so a retried message does not count the order twice.
func (b *BuyAgain) OnDelivered(ctx context.Context, tx *db.Tx, m events.Message) error {
	id, err := uuid.Parse(m.EntityID)
	if err != nil {
		return fmt.Errorf("catalog.buy_again: bad entity id: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO customer_products (user_id, shop_id, product_id, times, last_bought_at)
		SELECT o.customer_id, o.shop_id, i.product_id, 1, coalesce(o.delivered_at, now())
		FROM orders o JOIN order_items i ON i.order_id = o.id AND i.revision_no = o.revision_no
		WHERE o.id = $1 AND o.status = 'DELIVERED' AND i.state IN ('ACTIVE', 'SUBSTITUTE')
		GROUP BY o.customer_id, o.shop_id, i.product_id, o.delivered_at
		ON CONFLICT (user_id, shop_id, product_id) DO UPDATE
		SET times = customer_products.times + 1,
		    last_bought_at = greatest(customer_products.last_bought_at, EXCLUDED.last_bought_at)`, id); err != nil {
		return fmt.Errorf("catalog.buy_again: %w", err)
	}
	return nil
}
