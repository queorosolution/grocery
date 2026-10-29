package order

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/billing"
	"gp/internal/events"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/geo"
	"gp/internal/shared/money"
)

// reviseBill is T03. The shop's changes become a new revision of the order: every
// live line is copied into it (a removed one as REMOVED, a substitute as a new
// SUBSTITUTE line that points at the line it replaces), the money is worked out
// again by billing.Build from today's prices, and the revision timer starts.
// The earlier revision stays as it was, for the "before and after" view.
func reviseBill(ctx context.Context, s *Service, tx *db.Tx, o *orderRow, in TransitionInput) error {
	if in.Revise == nil {
		return apperr.Validation(map[string]string{"changes": "required"})
	}
	if err := in.Revise.check(); err != nil {
		return err
	}
	live, err := liveLines(ctx, tx, o.ID, o.RevisionNo)
	if err != nil {
		return err
	}
	p, err := planOf(live, in.Revise.Changes)
	if err != nil {
		return err
	}
	bill, err := s.reprice(ctx, tx, *o, p)
	if err != nil {
		return err
	}
	next := o.RevisionNo + 1
	if err := writeRevision(ctx, tx, *o, next, p, bill.Lines); err != nil {
		return err
	}
	slotID, err := s.moveSlot(ctx, tx, *o, in.Revise.SlotID)
	if err != nil {
		return err
	}
	t := bill.Totals
	if _, err := tx.Exec(ctx, `
		UPDATE orders SET items_cents = $2, delivery_fee_cents = $3, service_fee_cents = $4, discount_cents = $5, total_cents = $6,
			platform_margin_cents = $7, distance_m = $8, weight_g = $9, revision_no = $10, slot_id = $11
		WHERE id = $1`, o.ID, t.ItemsCents, t.DeliveryFeeCents, t.ServiceFeeCents, t.DiscountCents, t.TotalCents,
		bill.MarginCents, bill.DistanceM, bill.WeightG, next, slotID); err != nil {
		return err
	}
	o.RevisionNo, o.SlotID = next, slotID
	timeouts, err := s.shops.TimeoutsOf(ctx, tx, o.ShopID)
	if err != nil {
		return err
	}
	return s.addTimer(ctx, tx, *o, events.TimerRevision, timeouts.Revision)
}

// reprice works the new bill out. Prices, fees and weights are the shop's today;
// the address is the one on the order. A line that cannot be sold now is a
// refusal, so the shop fixes the change instead of sending the customer a bill
// with a hole in it.
func (s *Service) reprice(ctx context.Context, tx *db.Tx, o orderRow, p plan) (billing.Bill, error) {
	facts, err := s.shops.BillFacts(ctx, tx, o.ShopID)
	if err != nil {
		return billing.Bill{}, err
	}
	rule, err := s.delivery.RuleFor(ctx, tx, o.ShopID)
	if err != nil {
		return billing.Bill{}, err
	}
	ids := make([]uuid.UUID, len(p.inputs))
	for i, l := range p.inputs {
		ids[i] = l.ShopProductID
	}
	products, err := s.products.BillProducts(ctx, tx, o.ShopID, ids)
	if err != nil {
		return billing.Bill{}, err
	}
	var lat, lng float64
	if err := tx.QueryRow(ctx, `SELECT lat, lng FROM orders WHERE id = $1`, o.ID).Scan(&lat, &lng); err != nil {
		return billing.Bill{}, err
	}
	source := "DEFAULT"
	if rule.ShopID != nil {
		source = "SHOP"
	}
	bill, err := billing.Build(billing.BillInput{
		Shop: billing.ShopSnapshot{Location: facts.Location, MinOrderCents: money.Cents(facts.MinOrderCents),
			MinPlatformMarginCents: money.Cents(facts.MinPlatformMarginCents), MaxLines: facts.MaxLines, Open: true},
		Lines: p.inputs, Products: products, Address: geo.Point{Lat: lat, Lng: lng},
		FeeRule: rule.Rule, FeeRuleSource: source, Now: clock.Now(),
	})
	switch {
	case err != nil:
		return billing.Bill{}, err
	case len(bill.Excluded) > 0:
		return billing.Bill{}, apperr.New(apperr.ItemUnavailable)
	}
	return bill, nil
}

// moveSlot books the new window before it lets the old one go, so a full slot
// leaves the order where it was. It returns the slot the order ends in.
func (s *Service) moveSlot(ctx context.Context, tx *db.Tx, o orderRow, to *uuid.UUID) (*uuid.UUID, error) {
	switch {
	case to == nil:
		return o.SlotID, nil
	case o.SlotID == nil:
		return nil, apperr.Validation(map[string]string{"slotId": "not_used"})
	case *to == *o.SlotID:
		return o.SlotID, nil
	}
	if err := s.delivery.BookSlot(ctx, tx, o.ShopID, *to, clock.Now()); err != nil {
		return nil, err
	}
	return to, s.delivery.ReleaseSlot(ctx, tx, *o.SlotID)
}

// writeRevision copies the live lines into the new revision.
func writeRevision(ctx context.Context, tx *db.Tx, o orderRow, next int, p plan, lines []billing.BillLine) error {
	removedAs := map[uuid.UUID]uuid.UUID{}
	for _, old := range p.removed {
		id, err := copyRemoved(ctx, tx, old, next)
		if err != nil {
			return err
		}
		removedAs[old] = id
	}
	for _, l := range lines {
		state, replaces := lineActive, (*uuid.UUID)(nil)
		if old, isSub := p.replacement[l.ShopProductID]; isSub {
			replaced := removedAs[old]
			state, replaces = lineSubstitute, &replaced
		}
		if _, err := insertLine(ctx, tx, o.ID, o.ShopID, next, l, state, replaces); err != nil {
			return err
		}
	}
	return nil
}

// liveLines are the lines of one revision that still count.
func liveLines(ctx context.Context, tx *db.Tx, orderID uuid.UUID, revisionNo int) ([]liveLine, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, shop_product_id, qty::int, sub_pref FROM order_items
		WHERE order_id = $1 AND revision_no = $2 AND state IN ('ACTIVE', 'SUBSTITUTE') ORDER BY name, id`, orderID, revisionNo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []liveLine
	for rows.Next() {
		var l liveLine
		if err := rows.Scan(&l.ID, &l.ShopProductID, &l.Qty, &l.SubPref); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// copyRemoved writes a line again in the next revision as REMOVED, with no amount.
func copyRemoved(ctx context.Context, tx *db.Tx, oldID uuid.UUID, next int) (id uuid.UUID, err error) {
	err = tx.QueryRow(ctx, `
		INSERT INTO order_items (order_id, shop_id, shop_product_id, product_id, name, unit, weight_g, qty, mrp_cents,
			shop_price_cents, online_price_cents, line_cents, sub_pref, state, revision_no)
		SELECT order_id, shop_id, shop_product_id, product_id, name, unit, weight_g, qty, mrp_cents,
			shop_price_cents, online_price_cents, 0, sub_pref, 'REMOVED', $2
		FROM order_items WHERE id = $1 RETURNING id`, oldID, next).Scan(&id)
	return id, err
}
