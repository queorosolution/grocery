package refund

import (
	"context"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/finance"
	"gp/internal/order"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/money"
	"gp/internal/shared/validate"
)

// Decide approves or rejects a request (FR-RFD-03). Approving a shop's full cancel
// is T19: the order moves to CANCELLED_REFUND in the same transaction, and the
// move approves the refund. A request the system opened (money that cannot pay an
// order) is only approved: there is no paid order to cancel. Rejecting a partial
// request puts its line back on the bill.
func (s *Service) Decide(ctx context.Context, scope db.Scope, id uuid.UUID, in DecideInput) (Refund, error) {
	if err := validate.New().Check("approve", in.Approve != nil, "required").Text("note", in.Note, 0, maxReason).Err(); err != nil {
		return Refund{}, err
	}
	var out Refund
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		r, found, err := get(ctx, tx, id)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if r.Status != enum.RefundStatusRequested {
			return conflict("status", "not_requested")
		}
		switch {
		case !*in.Approve:
			if err = s.reject(ctx, tx, scope, r); err != nil {
				return err
			}
		case r.Type == enum.RefundTypeFull && r.RequestedBy != nil:
			if err = s.cancelOrder(ctx, tx, scope, r, in.Note); err != nil {
				return err
			}
		default:
			if _, err = tx.Exec(ctx, `UPDATE refunds SET status = 'APPROVED', decided_by = $2, decided_at = $3 WHERE id = $1`, id, scope.UserID, clock.Now()); err != nil {
				return err
			}
		}
		if err = audit.Write(ctx, tx, scope, audit.Entry{Action: "refund.decide", Entity: "refund", EntityID: id.String(), ShopID: r.ShopID,
			Before: map[string]any{"status": r.Status}, After: map[string]any{"approve": *in.Approve, "note": in.Note}}); err != nil {
			return err
		}
		out, _, err = read(ctx, tx, id)
		return err
	})
	return out, err
}

func (s *Service) reject(ctx context.Context, tx *db.Tx, scope db.Scope, r Refund) error {
	if _, err := tx.Exec(ctx, `UPDATE refunds SET status = 'REJECTED', decided_by = $2, decided_at = $3 WHERE id = $1`, r.ID, scope.UserID, clock.Now()); err != nil {
		return err
	}
	if r.Type == enum.RefundTypePartial && r.OrderItemID != nil {
		return s.orders.RestoreLine(ctx, tx, r.OrderID, *r.OrderItemID)
	}
	return nil
}

// cancelOrder is the approval of a shop's full cancel request. The order must
// still be where T19 can start from; if the driver already left, the admin rejects.
func (s *Service) cancelOrder(ctx context.Context, tx *db.Tx, scope db.Scope, r Refund, note string) error {
	o, err := s.orders.RefundOrder(ctx, tx, scope, r.OrderID)
	if err != nil {
		return err
	}
	if !order.Allowed(o.Status, enum.OrderStatusCancelledRefund, string(scope.Role)) {
		return apperr.New(apperr.InvalidTransition)
	}
	reason := note
	if reason == "" {
		reason = r.Reason
	}
	return s.orders.CancelRefund(ctx, tx, scope, r.OrderID, r.ID, reason)
}

// ApproveCancel is called by T19. With a refund id it approves that request.
// Without one (the admin cancels on their own) it writes an approved full refund
// of whatever has not been refunded yet, and refuses while another request is
// still open: those are decided first, so the customer is never refunded twice.
func (s *Service) ApproveCancel(ctx context.Context, tx *db.Tx, orderID, shopID, refundID uuid.UUID, totalCents money.Cents, actor uuid.UUID, reason string) error {
	if refundID != uuid.Nil {
		tag, err := tx.Exec(ctx, `UPDATE refunds SET status = 'APPROVED', decided_by = $3, decided_at = $4
			WHERE id = $1 AND order_id = $2 AND status = 'REQUESTED'`, refundID, orderID, actor, clock.Now())
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return conflict("status", "not_requested")
		}
		return nil
	}
	_, _, requested, refunded, err := openFor(ctx, tx, orderID, uuid.Nil)
	switch {
	case err != nil:
		return err
	case requested > 0:
		return conflict("refund", "pending")
	case totalCents-refunded <= 0:
		return nil // everything was refunded already
	}
	_, err = insert(ctx, tx, newRefund{OrderID: orderID, ShopID: shopID, Type: enum.RefundTypeFull, AmountCents: totalCents - refunded,
		Reason: reason, Status: enum.RefundStatusApproved, DecidedBy: actor})
	return err
}

// MarkPaidOut records that the money went back to the customer, with the
// reference of the transfer (FR-RFD-04). Only then does the ledger take it back:
// the shop's part and the platform's part of what was refunded (FR-FIN-02).
func (s *Service) MarkPaidOut(ctx context.Context, scope db.Scope, id uuid.UUID, in PaidOutInput) (Refund, error) {
	if err := validate.New().Str("reference", in.Reference, 3, 100).Err(); err != nil {
		return Refund{}, err
	}
	var out Refund
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		r, found, err := get(ctx, tx, id)
		if err != nil || !found {
			return apperr.NotFoundUnless(err, found)
		}
		if r.Status != enum.RefundStatusApproved {
			return conflict("status", "not_approved")
		}
		facts, err := s.ledgerFacts(ctx, tx, r)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE refunds SET status = 'PAID_OUT', reference = $2, paid_out_at = $3 WHERE id = $1`, id, in.Reference, clock.Now()); err != nil {
			return err
		}
		if facts != nil {
			if err = s.ledger.PostRefund(ctx, tx, *facts); err != nil {
				return err
			}
		}
		if err = audit.Write(ctx, tx, scope, audit.Entry{Action: "refund.paid_out", Entity: "refund", EntityID: id.String(), ShopID: r.ShopID,
			After: map[string]any{"amountCents": r.AmountCents}}); err != nil {
			return err
		}
		out, _, err = read(ctx, tx, id)
		return err
	})
	return out, err
}

// ledgerFacts is what the ledger takes back for this refund. A late payment never
// was in the ledger, so it takes back nothing (nil).
func (s *Service) ledgerFacts(ctx context.Context, tx *db.Tx, r Refund) (*finance.RefundFacts, error) {
	f := finance.RefundFacts{OrderID: r.OrderID, ShopID: r.ShopID, RefundID: r.ID}
	switch r.Type {
	case enum.RefundTypeFull:
		f.Full = true
	case enum.RefundTypePartial:
		line, err := s.orders.LineOf(ctx, tx, r.OrderID, *r.OrderItemID)
		if err != nil {
			return nil, err
		}
		f.ShopPartCents, f.PlatformPartCents = line.ShopPartCents, r.AmountCents-line.ShopPartCents
	default:
		return nil, nil
	}
	return &f, nil
}
