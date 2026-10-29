package stats

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/order"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// Orders is what stats read of the order module. *order.Service has it.
type Orders interface {
	StatFacts(ctx context.Context, tx *db.Tx, id uuid.UUID) (order.StatFacts, error)
	QueueCounts(ctx context.Context, tx *db.Tx, shopID uuid.UUID) (map[enum.OrderStatus]int, error)
	TopProducts(ctx context.Context, tx *db.Tx, shopID uuid.UUID, from, to time.Time, limit int) ([]order.TopProduct, error)
}

// payload is the part of an order event the stats need: the move it records.
type payload struct {
	From enum.OrderStatus `json:"from"`
	To   enum.OrderStatus `json:"to"`
}

// rule turns one event into the change it makes to the day's row. An event with
// no change returns the zero Sums.
type rule func(p payload, f order.StatFacts, at time.Time) Sums

// rules are the events the stats follow (docs/04 section 5). Approved and revised
// are here only for the review time.
var rules = map[enum.EventType]rule{
	enum.EventTypeOrderSubmitted:       func(payload, order.StatFacts, time.Time) Sums { return Sums{Submitted: 1} },
	enum.EventTypeOrderApproved:        review(Sums{}),
	enum.EventTypeOrderRevised:         review(Sums{}),
	enum.EventTypeOrderRejected:        review(Sums{Rejected: 1}),
	enum.EventTypeOrderExpired:         func(payload, order.StatFacts, time.Time) Sums { return Sums{Expired: 1} },
	enum.EventTypeOrderCancelled:       func(payload, order.StatFacts, time.Time) Sums { return Sums{Cancelled: 1} },
	enum.EventTypeOrderCancelledRefund: func(payload, order.StatFacts, time.Time) Sums { return Sums{Cancelled: 1} },
	enum.EventTypeOrderPaid:            paid,
	enum.EventTypeOrderStatusChanged:   picked,
	enum.EventTypeOrderDelivered:       func(payload, order.StatFacts, time.Time) Sums { return Sums{Delivered: 1} },
}

// seconds is a span in whole seconds, never negative (clocks of two hosts can differ a little).
func seconds(from, to time.Time) int64 { return max(0, int64(to.Sub(from)/time.Second)) }

// review adds the time the shop took to answer, but only for the first answer
// (SUBMITTED to anything): the customer's accept of a revision is not the shop's work.
func review(base Sums) rule {
	return func(p payload, f order.StatFacts, at time.Time) Sums {
		d := base // a copy: the rule runs for many events
		if p.From == enum.OrderStatusSubmitted {
			d.ReviewSeconds, d.ReviewCount = seconds(f.SubmittedAt, at), 1
		}
		return d
	}
}

// paid adds the money of T10. The shop's part of the items is what is left after
// the platform's margin; the platform earns the margin and the service fee.
func paid(_ payload, f order.StatFacts, _ time.Time) Sums {
	return Sums{
		Paid: 1, GMVCents: f.TotalCents, ShopItemsCents: f.ItemsCents - f.MarginCents,
		PlatformIncomeCents: f.MarginCents + f.ServiceFeeCents, DiscountCents: f.DiscountCents,
	}
}

// picked adds the picking time when the order reaches PACKED. PICKING alone changes nothing.
func picked(p payload, f order.StatFacts, at time.Time) Sums {
	if p.To != enum.OrderStatusPacked || f.PickingAt == nil {
		return Sums{}
	}
	return Sums{PickSeconds: seconds(*f.PickingAt, at), PickCount: 1}
}

// Service owns the stats tables.
type Service struct {
	db     *db.DB
	orders Orders
}

func NewService(d *db.DB, orders Orders) *Service { return &Service{db: d, orders: orders} }

// Register adds one consumer per followed event. The name is stored in
// processed_events, so a name never changes.
func (s *Service) Register(r *events.Registry) {
	for t := range rules {
		r.On(t, events.Consumer{Name: "stats." + string(t), Handle: s.Apply})
	}
}

// Apply adds one event to the row of its shop and Colombo day. It runs inside
// the dispatcher's transaction, which also holds the "processed" claim, so an
// event is counted once.
func (s *Service) Apply(ctx context.Context, tx *db.Tx, m events.Message) error {
	rule, ok := rules[m.Type]
	if !ok {
		return nil
	}
	id, err := uuid.Parse(m.EntityID)
	if err != nil {
		return nil // not an order event; nothing to count
	}
	var p payload
	if len(m.Data) > 0 {
		if err := json.Unmarshal(m.Data, &p); err != nil {
			return err
		}
	}
	f, err := s.orders.StatFacts(ctx, tx, id)
	if err != nil {
		return err
	}
	d := rule(p, f, m.At)
	if d == (Sums{}) {
		return nil
	}
	return add(ctx, tx, f.ShopID, m.At, d)
}
