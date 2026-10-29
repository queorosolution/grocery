package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/logx"
)

// Outgoing is one queue message.
type Outgoing struct {
	ID    string // the event id, so a failure can be matched to its event
	Body  []byte
	Delay time.Duration
}

// Queue sends messages to SQS. Send returns the ids that were NOT accepted.
type Queue interface {
	Send(ctx context.Context, batch []Outgoing) (failed []string, err error)
}

// Scheduler creates a one-time schedule. A schedule that already exists is not an error.
type Scheduler interface {
	CreateSchedule(ctx context.Context, t Timer) error
}

const (
	// siteRebuildDelay lets a burst of shop edits settle; the consumer skips all but the last.
	siteRebuildDelay = 120 * time.Second
	// maxBatch is the SQS limit for one SendMessageBatch.
	maxBatch = 10
	// minTimerLead keeps a schedule in the future: Scheduler refuses a time that has passed.
	minTimerLead = 30 * time.Second
)

// Publisher sends committed events and marks them sent.
type Publisher struct {
	db    *db.DB
	queue Queue
	sched Scheduler
}

func NewPublisher(d *db.DB, q Queue, s Scheduler) *Publisher {
	return &Publisher{db: d, queue: q, sched: s}
}

// Publish sends the messages: timers to Scheduler, the rest to the queue in
// batches of 10. Every event that was accepted gets sent_at; the others keep
// sent_at NULL, so the sweep tries again. It returns an error if any failed.
func (p *Publisher) Publish(ctx context.Context, msgs []Message) error {
	_, err := p.publish(ctx, msgs)
	return err
}

// publish is Publish that also says how many events were accepted.
func (p *Publisher) publish(ctx context.Context, msgs []Message) (int, error) {
	var sent []string
	var errs []error
	var batch []Outgoing
	flush := func() {
		if len(batch) == 0 {
			return
		}
		failed, err := p.queue.Send(ctx, batch)
		bad := map[string]bool{}
		for _, id := range failed {
			bad[id] = true
		}
		for _, o := range batch {
			if err == nil && !bad[o.ID] {
				sent = append(sent, o.ID)
			}
		}
		if err != nil {
			errs = append(errs, err)
		} else if len(failed) > 0 {
			errs = append(errs, fmt.Errorf("events: queue refused %d of %d messages", len(failed), len(batch)))
		}
		batch = batch[:0]
	}
	for _, m := range msgs {
		if m.Type == enum.EventTypeTimerSchedule {
			if err := p.schedule(ctx, m); err != nil {
				errs = append(errs, err)
				continue
			}
			sent = append(sent, m.ID)
			continue
		}
		body, err := json.Marshal(m)
		if err != nil {
			errs = append(errs, fmt.Errorf("events: encode %s: %w", m.ID, err))
			continue
		}
		o := Outgoing{ID: m.ID, Body: body}
		if m.Type == enum.EventTypeSiteRebuild {
			o.Delay = siteRebuildDelay
		}
		batch = append(batch, o)
		if len(batch) == maxBatch {
			flush()
		}
	}
	flush()
	if err := p.markSent(ctx, sent, msgs); err != nil {
		errs = append(errs, err)
	}
	return len(sent), errors.Join(errs...)
}

func (p *Publisher) schedule(ctx context.Context, m Message) error {
	var t Timer
	if err := json.Unmarshal(m.Data, &t); err != nil {
		return fmt.Errorf("events: timer %s: %w", m.ID, err)
	}
	if earliest := clock.Now().Add(minTimerLead); t.RunAt.Before(earliest) {
		t.RunAt = earliest // late is better than never: the handler is a no-op when the order moved on
	}
	return p.sched.CreateSchedule(ctx, t)
}

// markSent records the outcome in one statement: sent events get sent_at, and
// every event in this call counts one attempt.
func (p *Publisher) markSent(ctx context.Context, sent []string, msgs []Message) error {
	all := make([]string, len(msgs))
	for i, m := range msgs {
		all[i] = m.ID
	}
	if len(all) == 0 {
		return nil
	}
	err := p.db.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE outbox SET attempts = attempts + 1,
			       sent_at = CASE WHEN id = ANY($2) THEN $3 ELSE sent_at END
			WHERE id = ANY($1)`, all, sent, clock.Now())
		return err
	})
	if err != nil {
		logx.From(ctx).Error("events: mark sent failed", "err", err)
	}
	return err
}
