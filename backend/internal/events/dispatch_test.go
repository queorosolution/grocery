package events_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

func msg(id string) events.Message {
	return events.Message{ID: id, Type: enum.EventTypeOrderPaid, EntityID: "o1"}
}

// A queue may deliver twice (SQS is at-least-once): the consumer must run once.
func TestDispatch_DuplicateDeliveryRunsOnce(t *testing.T) {
	f := newFixture(t)
	var runs atomic.Int32
	reg := events.NewRegistry()
	reg.On(enum.EventTypeOrderPaid, events.Consumer{Name: "count", Handle: func(context.Context, *db.Tx, events.Message) error {
		runs.Add(1)
		return nil
	}})
	for i := 0; i < 3; i++ {
		if err := reg.Dispatch(context.Background(), f.env.DB, msg("evt_A")); err != nil {
			t.Fatal(err)
		}
	}
	if runs.Load() != 1 {
		t.Errorf("consumer ran %d times, want 1", runs.Load())
	}
	if err := reg.Dispatch(context.Background(), f.env.DB, msg("evt_B")); err != nil || runs.Load() != 2 {
		t.Errorf("another event must run: runs %d, err %v", runs.Load(), err)
	}
}

// Two Lambdas receive the same message at the same moment.
func TestDispatch_ConcurrentDuplicatesRunOnce(t *testing.T) {
	f := newFixture(t)
	var runs atomic.Int32
	reg := events.NewRegistry()
	reg.On(enum.EventTypeOrderPaid, events.Consumer{Name: "count", Handle: func(context.Context, *db.Tx, events.Message) error {
		runs.Add(1)
		return nil
	}})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := reg.Dispatch(context.Background(), f.env.DB, msg("evt_C")); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if runs.Load() != 1 {
		t.Errorf("consumer ran %d times, want 1", runs.Load())
	}
}

// A failed run must not use up the event: the queue delivers it again and it runs again.
// Its database changes are rolled back together with the claim.
func TestDispatch_FailureRollsBackAndRetries(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `CREATE TABLE IF NOT EXISTS probe (note text)`)
	f.exec(t, `GRANT ALL ON probe TO app_rw`)
	fail := true
	reg := events.NewRegistry()
	reg.On(enum.EventTypeOrderPaid, events.Consumer{Name: "flaky", Handle: func(ctx context.Context, tx *db.Tx, m events.Message) error {
		if _, err := tx.Exec(ctx, `INSERT INTO probe VALUES ($1)`, m.ID); err != nil {
			return err
		}
		if fail {
			return errBoom
		}
		return nil
	}})

	err := reg.Dispatch(context.Background(), f.env.DB, msg("evt_D"))
	if !errors.Is(err, errBoom) {
		t.Fatalf("first delivery: %v, want the consumer's error", err)
	}
	if f.count(t, `SELECT count(*) FROM probe`) != 0 || f.count(t, `SELECT count(*) FROM processed_events`) != 0 {
		t.Error("a failed run must leave neither its changes nor its claim")
	}
	fail = false
	if err := reg.Dispatch(context.Background(), f.env.DB, msg("evt_D")); err != nil {
		t.Fatal(err)
	}
	if f.count(t, `SELECT count(*) FROM probe`) != 1 || f.count(t, `SELECT count(*) FROM processed_events WHERE consumer = 'flaky'`) != 1 {
		t.Error("the retry must run once and leave its claim")
	}
}

// One broken consumer must not stop the others, and must not run its sibling twice on retry.
func TestDispatch_ConsumersAreIndependent(t *testing.T) {
	f := newFixture(t)
	var good, bad atomic.Int32
	failBad := true
	reg := events.NewRegistry()
	reg.On(enum.EventTypeOrderPaid, events.Consumer{Name: "bad", Handle: func(context.Context, *db.Tx, events.Message) error {
		bad.Add(1)
		if failBad {
			return errBoom
		}
		return nil
	}})
	reg.On(enum.EventTypeOrderPaid, events.Consumer{Name: "good", Handle: func(context.Context, *db.Tx, events.Message) error {
		good.Add(1)
		return nil
	}})
	if err := reg.Dispatch(context.Background(), f.env.DB, msg("evt_E")); !errors.Is(err, errBoom) {
		t.Fatalf("got %v", err)
	}
	failBad = false
	if err := reg.Dispatch(context.Background(), f.env.DB, msg("evt_E")); err != nil {
		t.Fatal(err)
	}
	if good.Load() != 1 || bad.Load() != 2 {
		t.Errorf("good ran %d times (want 1), bad ran %d times (want 2)", good.Load(), bad.Load())
	}
}

func TestDispatch_OtherTypesAndNoConsumers(t *testing.T) {
	f := newFixture(t)
	reg := events.NewRegistry()
	reg.On(enum.EventTypeOrderPaid, events.Consumer{Name: "x", Handle: func(context.Context, *db.Tx, events.Message) error {
		t.Error("must not run for another type")
		return nil
	}})
	m := msg("evt_F")
	m.Type = enum.EventTypeCatalogChanged
	if err := reg.Dispatch(context.Background(), f.env.DB, m); err != nil {
		t.Errorf("an event with no consumer is acknowledged: %v", err)
	}
}

func TestRegistry_OnRefusesMistakes(t *testing.T) {
	ok := func(context.Context, *db.Tx, events.Message) error { return nil }
	for name, fn := range map[string]func(*events.Registry){
		"twice": func(r *events.Registry) {
			r.On(enum.EventTypeOrderPaid, events.Consumer{Name: "a", Handle: ok})
			r.On(enum.EventTypeOrderPaid, events.Consumer{Name: "a", Handle: ok})
		},
		"unknown type": func(r *events.Registry) { r.On("order.invented", events.Consumer{Name: "a", Handle: ok}) },
		"no name":      func(r *events.Registry) { r.On(enum.EventTypeOrderPaid, events.Consumer{Handle: ok}) },
		"no handler":   func(r *events.Registry) { r.On(enum.EventTypeOrderPaid, events.Consumer{Name: "a"}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: want a panic at start-up", name)
				}
			}()
			fn(events.NewRegistry())
		}()
	}
	// The same name for another type is fine.
	r := events.NewRegistry()
	r.On(enum.EventTypeOrderPaid, events.Consumer{Name: "a", Handle: ok})
	r.On(enum.EventTypeOrderCancelled, events.Consumer{Name: "a", Handle: ok})
}
