// Package timerjob is the timer Lambda: EventBridge Scheduler calls it when an
// order deadline passes, and it lets the order module decide whether the order
// still has to expire. cmd/timer only wires this package to the Lambda runtime.
package timerjob

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/coupon"
	"gp/internal/delivery"
	"gp/internal/events"
	"gp/internal/events/eventsaws"
	"gp/internal/finance"
	"gp/internal/order"
	"gp/internal/shared/config"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
	"gp/internal/shared/secrets"
	"gp/internal/shop"
)

// Expirer is what the job needs of the order module.
type Expirer interface {
	Expire(ctx context.Context, orderID uuid.UUID, kind events.TimerKind, revisionNo int) (bool, error)
}

// Job handles one timer event.
type Job struct{ orders Expirer }

// New builds the job around the order module.
func New(orders Expirer) *Job { return &Job{orders: orders} }

// Wire builds the timer function's dependencies and returns a function that
// closes what it opened. An expiry moves an order, so its event goes to the
// queue after the commit like any other (and to the sweep if that fails). Only
// the shop, delivery and coupon modules are needed (an expiry gives back the
// place and the coupon): the other order dependencies belong to routes this
// function does not serve.
func Wire(ctx context.Context, cfg *config.Config, sec secrets.Store) (*Job, func(), error) {
	database, err := db.Connect(ctx, cfg, sec)
	if err != nil {
		return nil, nil, fmt.Errorf("wire: %w", err)
	}
	queue, sched, err := eventsaws.New(ctx, cfg)
	if err != nil {
		database.Close()
		return nil, nil, fmt.Errorf("wire: %w", err)
	}
	if queue != nil {
		events.SetPublisher(events.NewPublisher(database, queue, sched))
	}
	shopSvc := shop.NewService(database, sec, cfg.FeatureManualPay)
	orders := order.NewService(database, shopSvc, delivery.NewService(database, shopSvc), nil, nil, nil, coupon.NewService(database), finance.NewService())
	return New(orders), database.Close, nil
}

// timerEvent is the payload the scheduler sends (docs/07 section 10).
type timerEvent struct {
	OrderID    uuid.UUID        `json:"orderId"`
	Kind       events.TimerKind `json:"kind"`
	RevisionNo int              `json:"revisionNo"`
}

// Handle is the Lambda handler. The event is never logged. A malformed event can
// never succeed, so it is dropped instead of failing the invoke, which Lambda
// would only retry. A real failure (the database is down) is returned so it is
// retried; the retry is safe because Expire re-reads the order.
func (j *Job) Handle(ctx context.Context, raw json.RawMessage) error {
	var ev timerEvent
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&ev); err != nil || ev.OrderID == uuid.Nil || !ev.Kind.Valid() || ev.RevisionNo < 0 {
		logx.From(ctx).Error("timerjob.bad_event")
		return nil
	}
	moved, err := j.orders.Expire(ctx, ev.OrderID, ev.Kind, ev.RevisionNo)
	if err != nil {
		return fmt.Errorf("timerjob: expire: %w", err)
	}
	logx.From(ctx).Info("timerjob.fired", "kind", ev.Kind, "order", ev.OrderID, "expired", moved)
	return nil
}
