// Package events is the outbox: a service writes an event row in the same
// transaction as the change it announces, and the row is sent to SQS after the
// commit (docs/07 section 10). If the send is missed, the sweep sends it later.
// Consumers run in the worker, each at most once per event.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	"github.com/google/uuid"

	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/ids"
)

// Event is one thing that happened. Payload holds ids and small facts only:
// never customer data, because rows are kept and re-sent.
type Event struct {
	Type     enum.EventType
	ShopID   uuid.UUID // uuid.Nil for platform-wide events
	EntityID string
	Payload  any
}

// current is the publisher Add hands committed events to. It is set once at
// start-up; without one (local runs, most tests) events wait for the sweep.
var current atomic.Pointer[Publisher]

// SetPublisher sets the publisher used after commits. Pass nil to turn it off.
func SetPublisher(p *Publisher) { current.Store(p) }

// Add inserts the event into the outbox inside the caller's transaction and
// returns its id. After the transaction commits the event is published; if the
// transaction rolls back, the row is gone and nothing is ever sent.
func Add(ctx context.Context, tx *db.Tx, e Event) (string, error) {
	if !e.Type.Valid() {
		return "", fmt.Errorf("events: unknown type %q", e.Type)
	}
	payload := e.Payload
	if payload == nil {
		payload = map[string]any{}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("events: payload: %w", err)
	}
	m := Message{ID: ids.Event(), Type: e.Type, EntityID: e.EntityID, At: clock.Now(), Data: raw}
	if e.ShopID != uuid.Nil {
		m.ShopID = &e.ShopID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (id, type, shop_id, entity_id, payload, created_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5::jsonb, $6)`, m.ID, m.Type, m.ShopID, m.EntityID, string(raw), m.At)
	if err != nil {
		return "", err
	}
	tx.AfterCommit(func(ctx context.Context) error {
		if p := current.Load(); p != nil {
			return p.Publish(ctx, []Message{m})
		}
		return nil
	})
	return m.ID, nil
}
