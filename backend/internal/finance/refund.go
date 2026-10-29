package finance

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
)

// RefundFacts says what a refund that was paid out takes back from the ledger.
type RefundFacts struct {
	OrderID, ShopID, RefundID uuid.UUID
	// Full takes back whatever the order still holds, per party, so its rows add
	// up to 0 afterwards. Otherwise the two parts below are taken back: the shop's
	// share of the refunded line and the platform's (its margin on that line).
	Full                             bool
	ShopPartCents, PlatformPartCents money.Cents
}

// PostRefund writes the REFUND rows of a refund that was paid out (docs/05
// section 4). Nothing is edited: the old rows stay and the new ones are negative.
// An order that never reached PAID has no rows, so a full refund of it writes none
// (the money was never in the ledger).
func (s *Service) PostRefund(ctx context.Context, tx *db.Tx, f RefundFacts) error {
	var parts map[enum.LedgerParty]money.Cents
	if f.Full {
		held, err := heldBy(ctx, tx, f.OrderID)
		if err != nil {
			return err
		}
		parts = map[enum.LedgerParty]money.Cents{enum.LedgerPartyShop: -held[enum.LedgerPartyShop], enum.LedgerPartyPlatform: -held[enum.LedgerPartyPlatform]}
	} else {
		if f.ShopPartCents < 0 || f.PlatformPartCents < 0 {
			return errFacts
		}
		parts = map[enum.LedgerParty]money.Cents{enum.LedgerPartyShop: -f.ShopPartCents, enum.LedgerPartyPlatform: -f.PlatformPartCents}
	}
	// A fixed order, so two runs write the same rows in the same order.
	for _, party := range []enum.LedgerParty{enum.LedgerPartyShop, enum.LedgerPartyPlatform} {
		if parts[party] == 0 {
			continue
		}
		if err := insert(ctx, tx, f.ShopID, f.OrderID, entry{kind: enum.LedgerTypeRefund, party: party, amount: parts[party], refundID: &f.RefundID}); err != nil {
			return err
		}
	}
	return nil
}

// heldBy is what the order's rows add up to for each party.
func heldBy(ctx context.Context, tx *db.Tx, orderID uuid.UUID) (map[enum.LedgerParty]money.Cents, error) {
	rows, err := tx.Query(ctx, `SELECT party, sum(amount_cents)::bigint FROM ledger_entries WHERE order_id = $1 GROUP BY party`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	held := map[enum.LedgerParty]money.Cents{}
	for rows.Next() {
		var party enum.LedgerParty
		var sum money.Cents
		if err := rows.Scan(&party, &sum); err != nil {
			return nil, err
		}
		held[party] = sum
	}
	return held, rows.Err()
}
