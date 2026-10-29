package order

import (
	"context"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
)

// Quote prices the customer's cart as it would be billed now (FR-BILL-01).
// It writes nothing. Quantities come from the cart service, prices from our
// database, and the shop, the address and the slot are checked against the caller.
func (s *Service) Quote(ctx context.Context, scope db.Scope, in QuoteInput) (Quote, error) {
	if err := in.validate(); err != nil {
		return Quote{}, err
	}
	p, err := s.prepare(ctx, in.ShopID, in.CartID, in.Lines)
	if err != nil {
		return Quote{}, err
	}
	var out Quote
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		bill, err := s.price(ctx, tx, scope.UserID, in, p)
		if err != nil {
			return err
		}
		now := clock.Now()
		if out.Slots, err = s.delivery.OpenSlots(ctx, tx, in.ShopID, now); err != nil {
			return err
		}
		if in.SlotID != nil {
			if err = s.delivery.CheckSlot(ctx, tx, in.ShopID, *in.SlotID, now); err != nil {
				return err
			}
		}
		out.Bill = bill.Bill
		return nil
	})
	return out, err
}
