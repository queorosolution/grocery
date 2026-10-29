package order_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/enum"
	"gp/test/apptest"
)

// expire runs the timer as the timer function would and reports whether it moved the order.
func (f fixture) expire(id uuid.UUID, kind events.TimerKind, revision int) bool {
	f.a.T.Helper()
	moved, err := f.a.Orders.Expire(context.Background(), id, kind, revision)
	if err != nil {
		f.a.T.Fatalf("Expire(%s, %s, %d): %v", id, kind, revision, err)
	}
	return moved
}

func (f fixture) status(id uuid.UUID) string {
	f.a.T.Helper()
	var s string
	if err := f.a.Env.Owner.QueryRow(context.Background(), `SELECT status FROM orders WHERE id = $1`, id).Scan(&s); err != nil {
		f.a.T.Fatal(err)
	}
	return s
}

// Each deadline ends the wait it was made for (T05, T08, T11).
func TestTimer_Expires(t *testing.T) {
	cases := []struct {
		name     string
		kind     events.TimerKind
		revision int
		prepare  func(f fixture, shop apptest.Client, x trio)
	}{
		{"review", events.TimerReview, 0, func(fixture, apptest.Client, trio) {}},
		{"revision", events.TimerRevision, 1, func(f fixture, shop apptest.Client, x trio) {
			f.review(shop, x.o.ID, version(1, changes(qtyOf(x.rice0.ID, 2))))
		}},
		{"payment", events.TimerPayment, 0, func(f fixture, shop apptest.Client, x trio) {
			f.review(shop, x.o.ID, version(1, `"action":"APPROVE"`))
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			x := f.trio()
			c.prepare(f, f.staff("asha", "SHOP_STAFF"), x)
			if !f.expire(x.o.ID, c.kind, c.revision) {
				t.Fatal("the timer did not move the order")
			}
			if f.status(x.o.ID) != string(enum.OrderStatusExpired) || f.booked(x.slot) != 0 {
				t.Errorf("status %s, booked %d; want EXPIRED and the place released", f.status(x.o.ID), f.booked(x.slot))
			}
			if f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.expired' AND entity_id = $1`, x.o.ID.String()) != 1 ||
				f.a.Count(`SELECT count(*) FROM order_events WHERE order_id = $1 AND to_status = 'EXPIRED' AND actor_role = 'SYSTEM' AND actor_id IS NULL`, x.o.ID) != 1 {
				t.Error("the expiry is not in the order history or the outbox as a system move")
			}
			if f.a.Count(`SELECT count(*) FROM orders WHERE id = $1 AND closed_at IS NOT NULL`, x.o.ID) != 1 {
				t.Error("an expired order is not closed")
			}
			// A timer is delivered at least once: the repeat does nothing.
			if f.expire(x.o.ID, c.kind, c.revision) || f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.expired'`) != 1 {
				t.Error("the repeat of a timer expired the order again")
			}
		})
	}
}

// Timers never trust themselves (docs/03 section 4): an order that moved on stays.
func TestTimer_NoOpWhenMoved(t *testing.T) {
	f := newFixture(t)
	x := f.trio()
	staff := f.staff("asha", "SHOP_STAFF")
	id := x.o.ID

	// A wrong kind for the status, a wrong revision and an unknown order change nothing.
	for name, c := range map[string]struct {
		kind events.TimerKind
		rev  int
		id   uuid.UUID
	}{
		"payment timer on a submitted order": {events.TimerPayment, 0, id},
		"revision timer on a submitted one":  {events.TimerRevision, 0, id},
		"review timer of another revision":   {events.TimerReview, 1, id},
		"an order that does not exist":       {events.TimerReview, 0, uuid.New()},
	} {
		if f.expire(c.id, c.kind, c.rev) {
			t.Errorf("%s: the order was moved", name)
		}
	}
	if f.status(id) != "SUBMITTED" {
		t.Fatal("a stray timer moved the order")
	}

	// The shop answers first: the review timer of the first bill is stale from then on.
	f.review(staff, id, version(1, changes(qtyOf(x.rice0.ID, 2))))
	if f.expire(id, events.TimerReview, 0) || f.status(id) != "REVISED" {
		t.Error("the review timer moved a revised order")
	}
	f.me.OK("POST", "/orders/"+id.String()+"/accept-revision", `{"version":2,"revisionNo":1}`)
	if f.expire(id, events.TimerRevision, 1) || f.status(id) != "AWAITING_PAYMENT" {
		t.Error("the revision timer moved an order the customer accepted")
	}

	// Paid, cancelled and delivered orders keep their status and their place.
	for _, status := range []enum.OrderStatus{enum.OrderStatusPaid, enum.OrderStatusCancelled, enum.OrderStatusDelivered} {
		f.a.Exec(`UPDATE orders SET status = $2 WHERE id = $1`, id, status)
		if f.expire(id, events.TimerPayment, 1) || f.status(id) != string(status) {
			t.Errorf("the payment timer moved a %s order", status)
		}
	}
	if f.booked(x.slot) != 1 || f.a.Count(`SELECT count(*) FROM outbox WHERE type = 'order.expired'`) != 0 {
		t.Error("a no-op timer released the place or wrote an event")
	}

	// Only the three deadlines exist.
	if _, err := f.a.Orders.Expire(context.Background(), id, "other", 0); err == nil {
		t.Error("an unknown kind should be an error")
	}
}
