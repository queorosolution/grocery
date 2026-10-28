package events_test

import (
	"context"
	"testing"
	"time"

	"gp/internal/shared/enum"
)

func TestSweep_SendsOnlyOldUnsentEvents(t *testing.T) {
	f := newFixture(t)
	old := f.add(t, enum.EventTypeOrderPaid, nil)
	alreadySent := f.add(t, enum.EventTypeOrderPaid, nil)
	f.exec(t, `UPDATE outbox SET sent_at = now() WHERE id = $1`, alreadySent)
	f.age(t, 5*time.Minute)
	fresh := f.add(t, enum.EventTypeOrderPaid, nil) // younger than two minutes: its own publish may still be running

	res, err := f.pub.Sweep(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Found != 1 || res.Sent != 1 {
		t.Errorf("result = %+v, want 1 found and sent", res)
	}
	sent := f.queue.sent()
	if len(sent) != 1 || sent[0].ID != old {
		t.Fatalf("sent = %+v, want only %s", sent, old)
	}
	if f.count(t, `SELECT count(*) FROM outbox WHERE id = $1 AND sent_at IS NULL`, fresh) != 1 {
		t.Error("a fresh event must be left alone")
	}
	if f.count(t, `SELECT count(*) FROM outbox WHERE id = $1 AND sent_at IS NOT NULL`, old) != 1 {
		t.Error("the swept event must be marked sent")
	}
	// A second sweep finds nothing new.
	if res, _ := f.pub.Sweep(context.Background()); res.Found != 0 {
		t.Errorf("second sweep found %d", res.Found)
	}
}

func TestSweep_RecoversAfterAnOutage(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 3; i++ {
		f.add(t, enum.EventTypeOrderPaid, nil)
	}
	f.age(t, 5*time.Minute)
	f.queue.err = errBoom
	if _, err := f.pub.Sweep(context.Background()); err == nil {
		t.Fatal("an outage must be reported, so the run shows as failed")
	}
	if f.count(t, `SELECT count(*) FROM outbox WHERE sent_at IS NULL`) != 3 {
		t.Fatal("nothing may be marked sent during an outage")
	}
	f.queue.err = nil
	res, err := f.pub.Sweep(context.Background())
	if err != nil || res.Sent != 3 {
		t.Fatalf("after the outage: %+v, %v", res, err)
	}
}

func TestSweep_ResendsTimersToTheScheduler(t *testing.T) {
	f := newFixture(t)
	f.addTimer(t, newTimer())
	f.age(t, 5*time.Minute)
	if _, err := f.pub.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(f.sched.timers) != 1 || len(f.queue.sent()) != 0 {
		t.Errorf("timers %d, queue %d: a swept timer goes to the scheduler", len(f.sched.timers), len(f.queue.sent()))
	}
}

func TestSweep_WorksInBoundedRounds(t *testing.T) {
	f := newFixture(t)
	for i := 0; i < 130; i++ {
		f.add(t, enum.EventTypeCatalogChanged, nil)
	}
	f.age(t, 5*time.Minute)
	res, err := f.pub.Sweep(context.Background())
	if err != nil || res.Sent != 130 {
		t.Fatalf("result %+v, %v", res, err)
	}
}

func TestSweep_PrunesOldProcessedEvents(t *testing.T) {
	f := newFixture(t)
	f.exec(t, `INSERT INTO processed_events (event_id, consumer, at) VALUES
		('evt_old', 'c', now() - interval '8 days'), ('evt_new', 'c', now() - interval '6 days')`)
	res, err := f.pub.Sweep(context.Background())
	if err != nil || res.Pruned != 1 {
		t.Fatalf("result %+v, %v", res, err)
	}
	if f.count(t, `SELECT count(*) FROM processed_events WHERE event_id = 'evt_new'`) != 1 {
		t.Error("a row younger than 7 days must stay")
	}
}
