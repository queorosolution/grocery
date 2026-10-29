package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// What the dashboards read from orders (docs/06 section 8). The stats module
// reaches orders only through these functions, never through their tables.

// StatFacts are the numbers of one order that the daily stats add up.
type StatFacts struct {
	ShopID          uuid.UUID
	TotalCents      money.Cents
	ItemsCents      money.Cents
	MarginCents     money.Cents
	ServiceFeeCents money.Cents
	DiscountCents   money.Cents
	SubmittedAt     time.Time
	PickingAt       *time.Time
	PackedAt        *time.Time
}

// StatFacts reads them inside the caller's transaction (the stats consumers run
// as the system). An unknown order is NOT_FOUND.
func (s *Service) StatFacts(ctx context.Context, tx *db.Tx, id uuid.UUID) (StatFacts, error) {
	var f StatFacts
	err := tx.QueryRow(ctx, `
		SELECT shop_id, total_cents, items_cents, platform_margin_cents, service_fee_cents, discount_cents, submitted_at, picking_at, packed_at
		FROM orders WHERE id = $1`, id).
		Scan(&f.ShopID, &f.TotalCents, &f.ItemsCents, &f.MarginCents, &f.ServiceFeeCents, &f.DiscountCents, &f.SubmittedAt, &f.PickingAt, &f.PackedAt)
	if err != nil {
		return f, apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
	}
	return f, nil
}

// queueStatuses are the statuses in which an order waits for the shop or the driver.
var queueStatuses = []string{
	string(enum.OrderStatusSubmitted), string(enum.OrderStatusRevised), string(enum.OrderStatusAwaitingPayment),
	string(enum.OrderStatusPaid), string(enum.OrderStatusPicking), string(enum.OrderStatusPacked),
	string(enum.OrderStatusOutForDelivery), string(enum.OrderStatusDeliveryFailed),
}

// QueueCounts is how many of the shop's orders are in each open status now. A
// status with no order is 0.
func (s *Service) QueueCounts(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (map[enum.OrderStatus]int, error) {
	out := make(map[enum.OrderStatus]int, len(queueStatuses))
	for _, st := range queueStatuses {
		out[enum.OrderStatus(st)] = 0
	}
	rows, err := tx.Query(ctx, `SELECT status, count(*) FROM orders WHERE shop_id = $1 AND status = ANY($2) GROUP BY status`, shopID, queueStatuses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st enum.OrderStatus
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

// TopProduct is one line of the shop's best sellers.
type TopProduct struct {
	ProductID  uuid.UUID   `json:"productId"`
	Name       string      `json:"name"`
	Quantity   string      `json:"quantity"`
	SalesCents money.Cents `json:"salesCents"`
}

// soldStatuses are the statuses of an order that was paid and not given back.
var soldStatuses = []string{
	string(enum.OrderStatusPaid), string(enum.OrderStatusPicking), string(enum.OrderStatusPacked),
	string(enum.OrderStatusOutForDelivery), string(enum.OrderStatusDeliveryFailed), string(enum.OrderStatusDelivered),
}

// TopProducts are the shop's best sellers by sales, among the orders paid in
// [from, to). Only lines that still count (ACTIVE or SUBSTITUTE, current
// revision) are added. Quantity is text because it can hold grams or litres.
func (s *Service) TopProducts(ctx context.Context, tx *db.Tx, shopID uuid.UUID, from, to time.Time, limit int) ([]TopProduct, error) {
	rows, err := tx.Query(ctx, `
		SELECT i.product_id, (array_agg(i.name ORDER BY o.paid_at DESC))[1], sum(i.qty)::text, sum(i.line_cents)::bigint
		FROM orders o JOIN order_items i ON i.order_id = o.id AND i.revision_no = o.revision_no AND i.state IN ('ACTIVE', 'SUBSTITUTE')
		WHERE o.shop_id = $1 AND o.status = ANY($2) AND o.paid_at >= $3 AND o.paid_at < $4
		GROUP BY i.product_id ORDER BY 4 DESC, 1 LIMIT $5`, shopID, soldStatuses, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TopProduct{}
	for rows.Next() {
		var p TopProduct
		if err := rows.Scan(&p.ProductID, &p.Name, &p.Quantity, &p.SalesCents); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
