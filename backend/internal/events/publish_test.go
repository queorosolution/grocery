package events_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// The core promise (D17): an event written in a transaction that rolls back is never published.
func TestAdd_RolledBackEventIsNeverPublished(t *testing.T) {
	f := newFixture(t)
	f.publishing(t)
	err := f.env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		if _, err := events.Add(ctx, tx, events.Event{Type: enum.EventTypeOrderPaid, Payload: map[string]string{"orderId": "o1"}}); err != nil {
			return err
		}
		return errBoom
	})
	if err == nil {
		t.Fatal("the transaction should have failed")
	}
	if n := len(f.queue.sent()); n != 0 {
		t.Errorf("%d messages published from a rolled-back transaction", n)
	}
	if n := f.count(t, `SELECT count(*) FROM outbox`); n != 0 {
		t.Errorf("%d outbox rows survived the rollback", n)
	}
}

func TestAdd_CommittedEventIsPublishedOnceAndMarked(t *testing.T) {
	f := newFixture(t)
	f.publishing(t)
	id := f.add(t, enum.EventTypeOrderPaid, map[string]string{"orderId": "o1"})

	sent := f.queue.sent()
	if len(sent) != 1 || sent[0].ID != id || sent[0].Delay != 0 {
		t.Fatalf("sent = %+v", sent)
	}
	var m events.Message
	if err := json.Unmarshal(sent[0].Body, &m); err != nil {
		t.Fatal(err)
	}
	if m.ID != id || m.Type != enum.EventTypeOrderPaid || m.EntityID != "e1" || string(m.Data) != `{"orderId":"o1"}` || m.At.IsZero() {
		t.Errorf("message = %+v (data %s)", m, m.Data)
	}
	if n := f.count(t, `SELECT count(*) FROM outbox WHERE id = $1 AND sent_at IS NOT NULL AND attempts = 1`, id); n != 1 {
		t.Error("the row must be marked sent after one attempt")
	}
}

func TestAdd_WithoutPublisherWaitsForTheSweep(t *testing.T) {
	f := newFixture(t)
	id := f.add(t, enum.EventTypeOrderPaid, nil)
	if n := f.count(t, `SELECT count(*) FROM outbox WHERE id = $1 AND sent_at IS NULL AND payload = '{}'::jsonb`, id); n != 1 {
		t.Error("the row should wait, with an empty payload")
	}
	if len(f.queue.sent()) != 0 {
		t.Error("nothing should be sent without a publisher")
	}
}

func TestAdd_RefusesUnknownType(t *testing.T) {
	f := newFixture(t)
	err := f.env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		_, err := events.Add(ctx, tx, events.Event{Type: "order.invented"})
		return err
	})
	if err == nil {
		t.Fatal("an unknown type must be refused")
	}
}

func TestPublish_BatchesOfTenAndRebuildDelay(t *testing.T) {
	f := newFixture(t)
	var msgs []events.Message
	for i := 0; i < 24; i++ {
		id := f.add(t, enum.EventTypeCatalogChanged, nil)
		msgs = append(msgs, events.Message{ID: id, Type: enum.EventTypeCatalogChanged, Data: json.RawMessage(`{}`), At: time.Now()})
	}
	rebuild := f.add(t, enum.EventTypeSiteRebuild, nil)
	msgs = append(msgs, events.Message{ID: rebuild, Type: enum.EventTypeSiteRebuild, Data: json.RawMessage(`{}`), At: time.Now()})
	if err := f.pub.Publish(context.Background(), msgs); err != nil {
		t.Fatal(err)
	}
	var sizes []int
	for _, b := range f.queue.batches {
		sizes = append(sizes, len(b))
	}
	if fmt.Sprint(sizes) != "[10 10 5]" {
		t.Errorf("batch sizes = %v, want [10 10 5]", sizes)
	}
	for _, o := range f.queue.sent() {
		want := time.Duration(0)
		if o.ID == rebuild {
			want = 120 * time.Second
		}
		if o.Delay != want {
			t.Errorf("%s delay = %v, want %v", o.ID, o.Delay, want)
		}
	}
	if n := f.count(t, `SELECT count(*) FROM outbox WHERE sent_at IS NOT NULL`); n != 25 {
		t.Errorf("%d rows marked sent, want 25", n)
	}
}

func TestPublish_FailuresStayUnsentForTheSweep(t *testing.T) {
	f := newFixture(t)
	a := f.add(t, enum.EventTypeOrderPaid, nil)
	b := f.add(t, enum.EventTypeOrderPaid, nil)
	msgs := []events.Message{
		{ID: a, Type: enum.EventTypeOrderPaid, Data: json.RawMessage(`{}`)},
		{ID: b, Type: enum.EventTypeOrderPaid, Data: json.RawMessage(`{}`)},
	}

	f.queue.failIDs = map[string]bool{b: true}
	if err := f.pub.Publish(context.Background(), msgs); err == nil || !strings.Contains(err.Error(), "refused 1 of 2") {
		t.Fatalf("partial failure: %v", err)
	}
	if n := f.count(t, `SELECT count(*) FROM outbox WHERE id = $1 AND sent_at IS NOT NULL`, a); n != 1 {
		t.Error("the accepted event must be marked sent")
	}
	if n := f.count(t, `SELECT count(*) FROM outbox WHERE id = $1 AND sent_at IS NULL AND attempts = 1`, b); n != 1 {
		t.Error("the refused event must stay unsent and count one attempt")
	}

	f.queue.failIDs, f.queue.err = nil, errBoom
	if err := f.pub.Publish(context.Background(), msgs[1:]); err == nil {
		t.Fatal("a queue outage must be an error")
	}
	if n := f.count(t, `SELECT count(*) FROM outbox WHERE id = $1 AND sent_at IS NULL AND attempts = 2`, b); n != 1 {
		t.Error("an outage must leave the event unsent")
	}
}

func TestTimer_GoesToSchedulerNotQueue(t *testing.T) {
	f := newFixture(t)
	f.publishing(t)
	tm := newTimer()
	err := f.env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		_, err := events.AddTimer(ctx, tx, [16]byte{}, tm)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.queue.sent()) != 0 {
		t.Error("timers must not go to SQS")
	}
	if len(f.sched.timers) != 1 || f.sched.timers[0].OrderID != tm.OrderID || f.sched.timers[0].Kind != events.TimerPayment ||
		f.sched.timers[0].RevisionNo != 2 || !f.sched.timers[0].RunAt.Equal(tm.RunAt.Truncate(time.Nanosecond)) {
		t.Errorf("scheduled = %+v, want %+v", f.sched.timers, tm)
	}
	if want := fmt.Sprintf("ord-%s-payment-2", tm.OrderID); f.sched.timers[0].Name() != want {
		t.Errorf("name = %s, want %s", f.sched.timers[0].Name(), want)
	}
	if n := f.count(t, `SELECT count(*) FROM outbox WHERE type = 'timer.schedule' AND sent_at IS NOT NULL`); n != 1 {
		t.Error("the timer event must be marked sent")
	}
}

func TestTimer_PastDeadlineIsMovedForwardAndErrorsStayUnsent(t *testing.T) {
	f := newFixture(t)
	f.publishing(t)
	late := newTimer()
	late.RunAt = time.Now().Add(-time.Hour)
	f.addTimer(t, late)
	if got := f.sched.timers[0].RunAt; got.Before(time.Now().Add(20 * time.Second)) {
		t.Errorf("a past deadline must be moved at least ~30 s ahead, got %v", got)
	}

	f.sched.err = errBoom
	f.addTimer(t, newTimer())
	if n := f.count(t, `SELECT count(*) FROM outbox WHERE type = 'timer.schedule' AND sent_at IS NULL`); n != 1 {
		t.Errorf("%d unsent timers, want 1", n)
	}
}

func (f *fixture) addTimer(t *testing.T, tm events.Timer) {
	t.Helper()
	err := f.env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		_, err := events.AddTimer(ctx, tx, [16]byte{}, tm)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAddTimer_RefusesBadTimers(t *testing.T) {
	f := newFixture(t)
	for name, mutate := range map[string]func(*events.Timer){
		"no order":    func(tm *events.Timer) { tm.OrderID = [16]byte{} },
		"bad kind":    func(tm *events.Timer) { tm.Kind = "later" },
		"negative":    func(tm *events.Timer) { tm.RevisionNo = -1 },
		"no deadline": func(tm *events.Timer) { tm.RunAt = time.Time{} },
	} {
		tm := newTimer()
		mutate(&tm)
		err := f.env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
			_, err := events.AddTimer(ctx, tx, [16]byte{}, tm)
			return err
		})
		if err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
