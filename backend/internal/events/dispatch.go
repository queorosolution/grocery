package events

import (
	"context"
	"errors"
	"fmt"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

// Consumer reacts to one kind of event. Name is stored in processed_events, so
// it must never change once events have been processed. Handle gets the
// transaction that also holds the "processed" claim: its database changes and
// the claim commit together or not at all. External calls (SMS, HTTP) cannot be
// undone; if the commit fails after one, the retry repeats it, so keep them idempotent.
type Consumer struct {
	Name   string
	Handle func(ctx context.Context, tx *db.Tx, m Message) error
}

// Registry maps event types to consumers. Adding a consumer is one On call.
type Registry struct{ byType map[enum.EventType][]Consumer }

func NewRegistry() *Registry { return &Registry{byType: map[enum.EventType][]Consumer{}} }

// On registers a consumer. A repeated (type, name) pair or an unknown type is a
// programming error, so it panics at start-up instead of misbehaving later.
func (r *Registry) On(t enum.EventType, c Consumer) {
	if !t.Valid() || c.Name == "" || c.Handle == nil {
		panic(fmt.Sprintf("events: bad consumer %q for %q", c.Name, t))
	}
	for _, have := range r.byType[t] {
		if have.Name == c.Name {
			panic(fmt.Sprintf("events: consumer %q registered twice for %q", c.Name, t))
		}
	}
	r.byType[t] = append(r.byType[t], c)
}

// Dispatch runs every consumer of the message type, each at most once per event
// id. Each consumer has its own transaction, so one failing consumer neither
// blocks the others nor loses their claims. The first failure is returned after
// all have run, and the queue delivers the message again: consumers that
// already succeeded skip, the failed one runs again.
func (r *Registry) Dispatch(ctx context.Context, d *db.DB, m Message) error {
	var errs []error
	for _, c := range r.byType[m.Type] {
		err := d.Tx(ctx, db.System(), func(ctx context.Context, tx *db.Tx) error {
			// A concurrent duplicate waits here until this transaction ends: it
			// then skips after a commit and runs after a rollback.
			tag, err := tx.Exec(ctx, `
				INSERT INTO processed_events (event_id, consumer) VALUES ($1, $2)
				ON CONFLICT DO NOTHING`, m.ID, c.Name)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			return c.Handle(ctx, tx, m)
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("consumer %s: %w", c.Name, err))
		}
	}
	return errors.Join(errs...)
}
