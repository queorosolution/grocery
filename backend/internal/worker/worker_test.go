package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	lambdaevents "github.com/aws/aws-lambda-go/events"

	"gp/internal/events"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/worker"
	"gp/test/testdb"
)

type queue struct{ sent []events.Outgoing }

func (q *queue) Send(_ context.Context, b []events.Outgoing) ([]string, error) {
	q.sent = append(q.sent, b...)
	return nil, nil
}

type sched struct{}

func (sched) CreateSchedule(context.Context, events.Timer) error { return nil }

type rig struct {
	env  *testdb.Env
	w    *worker.Worker
	q    *queue
	runs map[string]*atomic.Int32
}

func newRig(t *testing.T) *rig {
	t.Helper()
	env := testdb.New(t)
	r := &rig{env: env, q: &queue{}, runs: map[string]*atomic.Int32{}}
	reg := events.NewRegistry()
	reg.On(enum.EventTypeOrderPaid, events.Consumer{Name: "count", Handle: func(_ context.Context, _ *db.Tx, m events.Message) error {
		if r.runs[m.ID] == nil {
			r.runs[m.ID] = &atomic.Int32{}
		}
		r.runs[m.ID].Add(1)
		if m.EntityID == "fail" {
			return errors.New("consumer failed")
		}
		return nil
	}})
	pub := events.NewPublisher(env.DB, r.q, sched{})
	r.w = worker.New(env.DB, reg, worker.Jobs(pub))
	return r
}

func record(id string, m events.Message) lambdaevents.SQSMessage {
	body, _ := json.Marshal(m)
	return lambdaevents.SQSMessage{MessageId: id, Body: string(body)}
}

func (r *rig) invoke(t *testing.T, v any) (lambdaevents.SQSEventResponse, error) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return r.w.Handle(context.Background(), raw)
}

func paid(id, entity string) events.Message {
	return events.Message{ID: id, Type: enum.EventTypeOrderPaid, EntityID: entity, At: time.Now()}
}

// SQS reports per message, so one bad message does not send the whole batch back.
func TestHandle_PartialBatchFailures(t *testing.T) {
	r := newRig(t)
	resp, err := r.invoke(t, lambdaevents.SQSEvent{Records: []lambdaevents.SQSMessage{
		record("m1", paid("evt_1", "ok")),
		record("m2", paid("evt_2", "fail")),
		{MessageId: "m3", Body: "not json"},
		{MessageId: "m4", Body: `{"id":"evt_4","type":"order.invented"}`},
		record("m5", paid("evt_5", "ok")),
	}})
	if err != nil {
		t.Fatal(err)
	}
	var failed []string
	for _, f := range resp.BatchItemFailures {
		failed = append(failed, f.ItemIdentifier)
	}
	if len(failed) != 3 || failed[0] != "m2" || failed[1] != "m3" || failed[2] != "m4" {
		t.Errorf("failures = %v, want [m2 m3 m4]", failed)
	}
	if r.runs["evt_1"].Load() != 1 || r.runs["evt_5"].Load() != 1 {
		t.Error("the good messages must still run")
	}
}

// The queue may deliver a message twice, in the same batch or later.
func TestHandle_DuplicateDeliveryRunsTheConsumerOnce(t *testing.T) {
	r := newRig(t)
	batch := lambdaevents.SQSEvent{Records: []lambdaevents.SQSMessage{record("m1", paid("evt_1", "ok")), record("m1b", paid("evt_1", "ok"))}}
	for i := 0; i < 2; i++ {
		resp, err := r.invoke(t, batch)
		if err != nil || len(resp.BatchItemFailures) != 0 {
			t.Fatalf("delivery %d: %+v, %v", i, resp, err)
		}
	}
	if got := r.runs["evt_1"].Load(); got != 1 {
		t.Errorf("consumer ran %d times for 4 deliveries, want 1", got)
	}
}

// A failed message comes back and succeeds on the retry.
func TestHandle_RetryAfterFailure(t *testing.T) {
	r := newRig(t)
	m := paid("evt_1", "fail")
	resp, _ := r.invoke(t, lambdaevents.SQSEvent{Records: []lambdaevents.SQSMessage{record("m1", m)}})
	if len(resp.BatchItemFailures) != 1 {
		t.Fatalf("first delivery should fail: %+v", resp)
	}
	m.EntityID = "ok"
	resp, _ = r.invoke(t, lambdaevents.SQSEvent{Records: []lambdaevents.SQSMessage{record("m1", m)}})
	if len(resp.BatchItemFailures) != 0 {
		t.Errorf("retry should pass: %+v", resp)
	}
}

func TestHandle_OutboxSweepJob(t *testing.T) {
	r := newRig(t)
	err := r.env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		_, err := events.Add(ctx, tx, events.Event{Type: enum.EventTypeOrderPaid})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.env.Owner.Exec(context.Background(), `UPDATE outbox SET created_at = created_at - interval '10 minutes'`); err != nil {
		t.Fatal(err)
	}
	if _, err := r.invoke(t, map[string]string{"type": "outbox.sweep"}); err != nil {
		t.Fatal(err)
	}
	if len(r.q.sent) != 1 {
		t.Errorf("the sweep sent %d messages, want 1", len(r.q.sent))
	}
	if _, err := r.invoke(t, map[string]string{"type": "stats.rollup"}); err != nil {
		t.Errorf("stats.rollup: %v", err)
	}
}

func TestHandle_BadInvokes(t *testing.T) {
	r := newRig(t)
	if _, err := r.invoke(t, map[string]string{"type": "outbox.explode"}); err == nil {
		t.Error("an unknown job must fail, so a wrong schedule is noticed")
	}
	if _, err := r.invoke(t, map[string]string{}); err == nil {
		t.Error("an event with neither records nor a job type must fail")
	}
	if _, err := r.w.Handle(context.Background(), json.RawMessage(`[1,2`)); err == nil {
		t.Error("unreadable input must fail")
	}
}
