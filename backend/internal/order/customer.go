package order

import (
	"context"
	"math"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// AcceptInput is the body of POST /orders/{id}/accept-revision. RevisionNo is
// the revision the customer was shown: a newer one has to be looked at again.
type AcceptInput struct {
	Version    int `json:"version"`
	RevisionNo int `json:"revisionNo"`
}

// CancelInput is the body of POST /orders/{id}/cancel.
type CancelInput struct {
	Version int `json:"version"`
}

// AcceptRevision is T07: the customer agrees to the shop's revised bill.
func (s *Service) AcceptRevision(ctx context.Context, scope db.Scope, orderID uuid.UUID, in AcceptInput) (Detail, error) {
	if err := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Int("revisionNo", int64(in.RevisionNo), 1, math.MaxInt32).Err(); err != nil {
		return Detail{}, err
	}
	return s.Transition(ctx, scope, orderID, enum.OrderStatusAwaitingPayment, TransitionInput{Version: in.Version, ExpectRevision: &in.RevisionNo})
}

// Cancel is T06, T09 or T12: the customer withdraws before paying. The target is
// always CANCELLED; from PAID on the table has no such move, so it is refused
// there as INVALID_TRANSITION (BIZ-10) and not by a check of its own.
func (s *Service) Cancel(ctx context.Context, scope db.Scope, orderID uuid.UUID, in CancelInput) (Detail, error) {
	if err := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Err(); err != nil {
		return Detail{}, err
	}
	return s.Transition(ctx, scope, orderID, enum.OrderStatusCancelled, TransitionInput{Version: in.Version})
}
