package order

import (
	"context"
	"math"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// StatusInput is the body of POST /console/shops/{shopId}/orders/{id}/status.
type StatusInput struct {
	Version int              `json:"version"`
	To      enum.OrderStatus `json:"to"`
}

// fulfilTargets are the statuses the shop moves an order to: T13 picking, T14
// packed, and T15 / T18 out for delivery (the same target; the table tells a
// first send from a retry by the status the order is in).
var fulfilTargets = map[enum.OrderStatus]bool{
	enum.OrderStatusPicking: true, enum.OrderStatusPacked: true, enum.OrderStatusOutForDelivery: true,
}

// Fulfil moves an order along the shop's side of the flow (FR-DLV-01). Everything
// else about the move is Transition's: the table, the scope, the version.
func (s *Service) Fulfil(ctx context.Context, scope db.Scope, orderID uuid.UUID, in StatusInput) (Detail, error) {
	if err := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Check("to", fulfilTargets[in.To], "invalid").Err(); err != nil {
		return Detail{}, err
	}
	return s.Transition(ctx, scope, orderID, in.To, TransitionInput{Version: in.Version})
}
