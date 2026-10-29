package order

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"gp/internal/billing"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/ids"
)

// orderRow is the part of an order the transition core and the hooks work on.
type orderRow struct {
	ID         uuid.UUID
	Code       string
	ShopID     uuid.UUID
	CustomerID uuid.UUID
	DriverID   *uuid.UUID
	Status     enum.OrderStatus
	Version    int
	RevisionNo int
	SlotID     *uuid.UUID
}

// lockOrder reads and locks the order. What the caller's scope cannot see (row
// level security) is simply not found.
func lockOrder(ctx context.Context, tx *db.Tx, id uuid.UUID) (orderRow, bool, error) {
	var o orderRow
	err := tx.QueryRow(ctx, `
		SELECT id, code, shop_id, customer_id, driver_id, status, version, revision_no, slot_id
		FROM orders WHERE id = $1 FOR UPDATE`, id).
		Scan(&o.ID, &o.Code, &o.ShopID, &o.CustomerID, &o.DriverID, &o.Status, &o.Version, &o.RevisionNo, &o.SlotID)
	return o, err == nil, db.IgnoreNoRows(err)
}

// newOrder is everything T01 writes. The money comes from billing.Build only.
type newOrder struct {
	ShopID, CustomerID uuid.UUID
	ShopSlug           string
	DeliveryType       enum.DeliveryType
	PaymentProvider    enum.PaymentProvider
	Address            addressSnapshot
	Lat, Lng           float64
	SlotID             *uuid.UUID
	Note               *string
	CartRef            string
	Bill               billing.Bill
}

// addressSnapshot is the address as it was when the bill was sent: later edits
// of the saved address never change an order.
type addressSnapshot struct {
	Label    string  `json:"label"`
	Line1    string  `json:"line1"`
	Landmark *string `json:"landmark"`
	Phone    string  `json:"phone"`
}

// insertCodeTries is how often a clash on the order code is retried. The code
// has 32^6 values per shop prefix, so one clash is rare and two almost impossible.
const insertCodeTries = 3

// insertOrder writes the order as SUBMITTED. A clash on the unique code is
// retried with a new code; ON CONFLICT keeps the transaction usable.
func insertOrder(ctx context.Context, tx *db.Tx, n newOrder) (id uuid.UUID, code string, err error) {
	t := n.Bill.Totals
	address, err := json.Marshal(n.Address)
	if err != nil {
		return uuid.Nil, "", err
	}
	for i := 0; i < insertCodeTries; i++ {
		code = ids.OrderCode(n.ShopSlug)
		err = tx.QueryRow(ctx, `
			INSERT INTO orders (code, shop_id, customer_id, status, delivery_type, address, lat, lng, distance_m, weight_g,
				slot_id, note, cart_ref, items_cents, delivery_fee_cents, service_fee_cents, discount_cents, total_cents,
				platform_margin_cents, payment_provider)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
			ON CONFLICT (code) DO NOTHING RETURNING id`,
			code, n.ShopID, n.CustomerID, enum.OrderStatusSubmitted, n.DeliveryType, string(address), n.Lat, n.Lng,
			n.Bill.DistanceM, n.Bill.WeightG, n.SlotID, n.Note, n.CartRef, t.ItemsCents, t.DeliveryFeeCents,
			t.ServiceFeeCents, t.DiscountCents, t.TotalCents, n.Bill.MarginCents, n.PaymentProvider).Scan(&id)
		if !errors.Is(err, pgx.ErrNoRows) {
			return id, code, err
		}
	}
	return uuid.Nil, "", fmt.Errorf("order: no free order code after %d tries", insertCodeTries)
}

// Line states (order_items.state). A live line is ACTIVE or SUBSTITUTE.
const (
	lineActive     = "ACTIVE"
	lineRemoved    = "REMOVED"
	lineSubstitute = "SUBSTITUTE"
)

// insertItems writes the lines of the bill, revision 0. The product id and the
// MRP come from the shop product itself; the prices are the ones Build used.
func insertItems(ctx context.Context, tx *db.Tx, orderID, shopID uuid.UUID, lines []billing.BillLine) error {
	for _, l := range lines {
		if _, err := insertLine(ctx, tx, orderID, shopID, 0, l, lineActive, nil); err != nil {
			return err
		}
	}
	return nil
}

// insertLine writes one priced line into a revision of the order. replaces is
// the removed line a substitute stands in for.
func insertLine(ctx context.Context, tx *db.Tx, orderID, shopID uuid.UUID, revisionNo int, l billing.BillLine, state string, replaces *uuid.UUID) (id uuid.UUID, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO order_items (order_id, shop_id, shop_product_id, product_id, name, unit, weight_g, qty, mrp_cents,
			shop_price_cents, online_price_cents, line_cents, sub_pref, state, replaces_item_id, revision_no)
		SELECT $1, sp.shop_id, sp.id, sp.product_id, $3, $4, $5, $6, sp.mrp_cents, $7, $8, $9, $10, $12, $13, $14
		FROM shop_products sp WHERE sp.id = $2 AND sp.shop_id = $11 RETURNING id`,
		orderID, l.ShopProductID, l.Name, l.Unit, l.WeightG, l.Qty, l.ShopPriceCents, l.UnitPriceCents, l.LineCents, l.SubPref, shopID,
		state, replaces, revisionNo).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("order: shop product %s is not sold by shop %s", l.ShopProductID, shopID)
	}
	return id, err
}

// lockCustomer serialises one customer's submits, so two at once cannot both
// pass the "fewer than 3 open orders" check.
func lockCustomer(ctx context.Context, tx *db.Tx, userID uuid.UUID) error {
	var id uuid.UUID
	// A closed or blocked account may still hold a valid access token for a few minutes.
	err := tx.QueryRow(ctx, `SELECT id FROM users WHERE id = $1 AND status = 'ACTIVE' FOR UPDATE`, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.New(apperr.Unauthenticated)
	}
	return err
}

// openUnpaid counts the orders the customer has not paid for yet and that are still alive.
func openUnpaid(ctx context.Context, tx *db.Tx, userID uuid.UUID) (n int, err error) {
	err = tx.QueryRow(ctx, `SELECT count(*) FROM orders WHERE customer_id = $1 AND status = ANY($2)`,
		userID, []string{string(enum.OrderStatusSubmitted), string(enum.OrderStatusRevised), string(enum.OrderStatusAwaitingPayment)}).Scan(&n)
	return n, err
}

// writeEvent appends one row to the order's history.
func writeEvent(ctx context.Context, tx *db.Tx, o orderRow, from *enum.OrderStatus, to enum.OrderStatus, actor actor, reason string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO order_events (order_id, shop_id, from_status, to_status, actor_id, actor_role, reason)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''))`, o.ID, o.ShopID, from, to, actor.id, actor.role, reason)
	return err
}
