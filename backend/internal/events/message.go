package events

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/enum"
)

// Message is what travels on the queue and what consumers receive (docs/04 section 5).
type Message struct {
	ID       string          `json:"id"`
	Type     enum.EventType  `json:"type"`
	ShopID   *uuid.UUID      `json:"shopId"`
	EntityID string          `json:"entityId,omitempty"`
	At       time.Time       `json:"at"`
	Data     json.RawMessage `json:"data"`
}
