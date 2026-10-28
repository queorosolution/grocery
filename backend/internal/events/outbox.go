// Package events is the outbox: a service writes an event row in the same
// transaction as the change it announces (docs/07 §10). Sending, retries and
// consumers arrive with the worker (P12); this file is the write side that
// every module needs from the start.
package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

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

// Add inserts the event into the outbox inside the caller's transaction and
// returns its id.
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
	var shop *uuid.UUID
	if e.ShopID != uuid.Nil {
		shop = &e.ShopID
	}
	id := ids.Event()
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (id, type, shop_id, entity_id, payload)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5::jsonb)`, id, e.Type, shop, e.EntityID, string(raw))
	return id, err
}
