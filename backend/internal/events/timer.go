package events

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// TimerKind says which deadline a timer is.
type TimerKind string

const (
	TimerReview   TimerKind = "review"
	TimerRevision TimerKind = "revision"
	TimerPayment  TimerKind = "payment"
)

func (k TimerKind) valid() bool {
	return k == TimerReview || k == TimerRevision || k == TimerPayment
}

// Timer is a one-time deadline for an order. RevisionNo is part of the schedule
// name, so a new revision gets a new schedule and the old one fires as a no-op.
type Timer struct {
	OrderID    uuid.UUID `json:"orderId"`
	Kind       TimerKind `json:"kind"`
	RevisionNo int       `json:"revisionNo"`
	RunAt      time.Time `json:"runAt"`
}

// Name is the schedule name. Creating the same name twice is not an error
// (ConflictException counts as success), which makes the publish repeatable.
func (t Timer) Name() string {
	return fmt.Sprintf("ord-%s-%s-%d", t.OrderID, t.Kind, t.RevisionNo)
}

// AddTimer writes a timer.schedule event in the caller's transaction. The
// publisher creates the schedule after the commit; timers never go to SQS.
func AddTimer(ctx context.Context, tx *db.Tx, shopID uuid.UUID, t Timer) (string, error) {
	if t.OrderID == uuid.Nil || !t.Kind.valid() || t.RevisionNo < 0 || t.RunAt.IsZero() {
		return "", fmt.Errorf("events: bad timer %+v", t)
	}
	t.RunAt = t.RunAt.UTC()
	return Add(ctx, tx, Event{Type: enum.EventTypeTimerSchedule, ShopID: shopID, EntityID: t.OrderID.String(), Payload: t})
}
