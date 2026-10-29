package order

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// timerMove is what a deadline does to an order: the status it waits in, and
// the move (T05, T08, T11) that ends the wait.
type timerMove struct {
	waiting enum.OrderStatus
	to      enum.OrderStatus
}

var timerMoves = map[events.TimerKind]timerMove{
	events.TimerReview:   {enum.OrderStatusSubmitted, enum.OrderStatusExpired},
	events.TimerRevision: {enum.OrderStatusRevised, enum.OrderStatusExpired},
	events.TimerPayment:  {enum.OrderStatusAwaitingPayment, enum.OrderStatusExpired},
}

// Expire runs when a timer fires. It never trusts the timer (docs/03 section 4):
// it reads the order again and moves it only if it is still waiting in the status
// and revision the timer was made for. Anything else, a paid order, a cancelled
// one, a newer revision, an order that is gone, is a no-op and not an error, so
// the schedule is not retried. It reports whether the order was moved.
func (s *Service) Expire(ctx context.Context, orderID uuid.UUID, kind events.TimerKind, revisionNo int) (bool, error) {
	m, ok := timerMoves[kind]
	if !ok {
		return false, fmt.Errorf("order: unknown timer kind %q", kind)
	}
	moved := false
	err := s.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		moved = false
		o, found, err := lockOrder(ctx, tx, orderID)
		if err != nil || !found || o.Status != m.waiting || o.RevisionNo != revisionNo {
			return err
		}
		if err := s.transition(ctx, tx, db.System(), orderID, m.to, TransitionInput{Version: o.Version}); err != nil {
			return err
		}
		moved = true
		return nil
	})
	return moved && err == nil, err
}
