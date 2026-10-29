package events_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/test/testdb"
)

type fakeQueue struct {
	mu      sync.Mutex
	batches [][]events.Outgoing
	failIDs map[string]bool // refused by SQS
	err     error           // the whole call fails
}

func (q *fakeQueue) Send(_ context.Context, batch []events.Outgoing) ([]string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return nil, q.err
	}
	q.batches = append(q.batches, append([]events.Outgoing(nil), batch...))
	var failed []string
	for _, o := range batch {
		if q.failIDs[o.ID] {
			failed = append(failed, o.ID)
		}
	}
	return failed, nil
}

func (q *fakeQueue) sent() []events.Outgoing {
	q.mu.Lock()
	defer q.mu.Unlock()
	var all []events.Outgoing
	for _, b := range q.batches {
		all = append(all, b...)
	}
	return all
}

type fakeScheduler struct {
	mu     sync.Mutex
	timers []events.Timer
	err    error
}

func (s *fakeScheduler) CreateSchedule(_ context.Context, t events.Timer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.timers = append(s.timers, t)
	return nil
}

type fixture struct {
	env   *testdb.Env
	queue *fakeQueue
	sched *fakeScheduler
	pub   *events.Publisher
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	env := testdb.New(t)
	f := &fixture{env: env, queue: &fakeQueue{}, sched: &fakeScheduler{}}
	f.pub = events.NewPublisher(env.DB, f.queue, f.sched)
	return f
}

// publishing turns on the after-commit publish for the test, then off again.
func (f *fixture) publishing(t *testing.T) {
	t.Helper()
	events.SetPublisher(f.pub)
	t.Cleanup(func() { events.SetPublisher(nil) })
}

// add writes one event in its own committed transaction and returns its id.
func (f *fixture) add(t *testing.T, typ enum.EventType, payload any) string {
	t.Helper()
	var id string
	err := f.env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) (err error) {
		id, err = events.Add(ctx, tx, events.Event{Type: typ, EntityID: "e1", Payload: payload})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.env.Owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.env.Owner.Exec(context.Background(), query, args...); err != nil {
		t.Fatal(err)
	}
}

// age makes every outbox row look older than the sweep age.
func (f *fixture) age(t *testing.T, d time.Duration) {
	t.Helper()
	f.exec(t, `UPDATE outbox SET created_at = created_at - $1::interval`, d.String())
}

var errBoom = errors.New("boom")

func newTimer() events.Timer {
	return events.Timer{OrderID: uuid.New(), Kind: events.TimerPayment, RevisionNo: 2, RunAt: time.Now().Add(time.Hour)}
}
