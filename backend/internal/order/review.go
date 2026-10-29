package order

import (
	"context"
	"math"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// ReviewInput is the body of POST /console/shops/{shopId}/orders/{id}/review
// (docs/06 section 8). Action is APPROVE, REVISE or REJECT.
type ReviewInput struct {
	Version int        `json:"version"`
	Action  Action     `json:"action"`
	Reason  string     `json:"reason"`
	SlotID  *uuid.UUID `json:"slotId"`
	Changes []Change   `json:"changes"`
}

// Review is the shop's answer to a submitted bill. It only checks that the body
// fits the action and then makes the move through Transition, which decides
// whether this caller may and whether the order is in a state to be reviewed.
func (s *Service) Review(ctx context.Context, scope db.Scope, orderID uuid.UUID, in ReviewInput) (Detail, error) {
	to, move, err := in.move()
	if err != nil {
		return Detail{}, err
	}
	return s.Transition(ctx, scope, orderID, to, move)
}

// move turns the body into the status to move to and what Transition needs.
// Fields that do not belong to the action are refused, not ignored, so a shop
// cannot think it changed something that was dropped.
func (in ReviewInput) move() (enum.OrderStatus, TransitionInput, error) {
	v := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32)
	move := TransitionInput{Version: in.Version}
	var to enum.OrderStatus
	switch in.Action {
	case ActionApprove:
		to = enum.OrderStatusAwaitingPayment
		v.Check("reason", in.Reason == "", "not_used").Check("slotId", in.SlotID == nil, "not_used").Check("changes", len(in.Changes) == 0, "not_used")
	case ActionReject:
		to, move.Reason = enum.OrderStatusRejected, in.Reason
		v.Str("reason", in.Reason, 3, maxReason).Check("slotId", in.SlotID == nil, "not_used").Check("changes", len(in.Changes) == 0, "not_used")
	case ActionRevise:
		to, move.Reason, move.Revise = enum.OrderStatusRevised, in.Reason, &Revision{Changes: in.Changes, SlotID: in.SlotID}
		if in.Reason != "" {
			v.Str("reason", in.Reason, 3, maxReason)
		}
	default:
		v.Check("action", false, "invalid")
	}
	return to, move, v.Err()
}
