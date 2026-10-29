package order

import (
	"context"
	"time"

	"gp/internal/events"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// hook is the side effect of one move (docs/07 section 9 step 4). It runs in the
// transition's transaction, before the status is saved, so a failing hook rolls
// the move back.
type hook func(ctx context.Context, s *Service, tx *db.Tx, o *orderRow, in TransitionInput) error

// hooks is keyed by transition id. A move with no entry cannot run: Transition
// refuses it, so a half-built flow can never change an order without its side
// effects. Each later prompt adds its moves here.
var hooks = map[string]hook{
	"T01": startReviewTimer,
	// The bill is final: freeze it and start the payment timer.
	"T02": approve, "T07": approve,
	"T03": reviseBill,
	// Release the slot place. An order that waited for payment also gives back its coupon.
	"T04": releaseSlot, "T05": releaseSlot, "T06": releaseSlot, "T08": releaseSlot, "T09": releaseSlot,
	"T11": releaseUnpaid, "T12": releaseUnpaid, "T19": releaseSlot,
}

// startReviewTimer is T01: the shop has the review timeout to answer.
func startReviewTimer(ctx context.Context, s *Service, tx *db.Tx, o *orderRow, _ TransitionInput) error {
	t, err := s.shops.TimeoutsOf(ctx, tx, o.ShopID)
	if err != nil {
		return err
	}
	return s.addTimer(ctx, tx, *o, events.TimerReview, t.Review)
}

func releaseSlot(ctx context.Context, s *Service, tx *db.Tx, o *orderRow, _ TransitionInput) error {
	if o.SlotID == nil {
		return nil // a courier order has no slot
	}
	return s.delivery.ReleaseSlot(ctx, tx, *o.SlotID)
}

// releaseUnpaid is T11 and T12: the order was never paid, so both the place and
// the coupon use it held go back.
func releaseUnpaid(ctx context.Context, s *Service, tx *db.Tx, o *orderRow, in TransitionInput) error {
	if err := releaseSlot(ctx, s, tx, o, in); err != nil {
		return err
	}
	_, err := s.coupons.Release(ctx, tx, o.ID)
	return err
}

// approve is T02 and T07: the totals stop changing and the customer has the
// payment timeout to pay (docs/03 section 6). The numbers themselves are the
// ones the customer saw: the bill of the submit, or the shop's revision.
func approve(ctx context.Context, s *Service, tx *db.Tx, o *orderRow, _ TransitionInput) error {
	if _, err := tx.Exec(ctx, `UPDATE orders SET frozen_at = $2 WHERE id = $1`, o.ID, clock.Now()); err != nil {
		return err
	}
	t, err := s.shops.TimeoutsOf(ctx, tx, o.ShopID)
	if err != nil {
		return err
	}
	return s.addTimer(ctx, tx, *o, events.TimerPayment, t.Payment)
}

func (s *Service) addTimer(ctx context.Context, tx *db.Tx, o orderRow, kind events.TimerKind, after time.Duration) error {
	_, err := events.AddTimer(ctx, tx, o.ShopID, events.Timer{OrderID: o.ID, Kind: kind, RevisionNo: o.RevisionNo, RunAt: clock.Now().Add(after)})
	return err
}

// eventFor is the outbox event of a move, by the status it ends in (docs/04 section 5).
func eventFor(to enum.OrderStatus) enum.EventType {
	switch to {
	case enum.OrderStatusSubmitted:
		return enum.EventTypeOrderSubmitted
	case enum.OrderStatusAwaitingPayment:
		return enum.EventTypeOrderApproved
	case enum.OrderStatusRevised:
		return enum.EventTypeOrderRevised
	case enum.OrderStatusRejected:
		return enum.EventTypeOrderRejected
	case enum.OrderStatusExpired:
		return enum.EventTypeOrderExpired
	case enum.OrderStatusCancelled:
		return enum.EventTypeOrderCancelled
	case enum.OrderStatusPaid:
		return enum.EventTypeOrderPaid
	case enum.OrderStatusOutForDelivery:
		return enum.EventTypeOrderOutForDelivery
	case enum.OrderStatusDelivered:
		return enum.EventTypeOrderDelivered
	case enum.OrderStatusDeliveryFailed:
		return enum.EventTypeOrderDeliveryFailed
	case enum.OrderStatusCancelledRefund:
		return enum.EventTypeOrderCancelledRefund
	}
	return enum.EventTypeOrderStatusChanged // PICKING, PACKED
}
