package order

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/events"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
)

const (
	// systemActor is the actor of timers and webhooks. It is not a Role.
	systemActor = "SYSTEM"
	maxReason   = 300
)

// actor is who makes a move, taken from the scope the route guard built.
type actor struct {
	id   *uuid.UUID
	role string
}

func actorOf(sc db.Scope) actor {
	a := actor{role: string(sc.Role)}
	if sc.UserID != uuid.Nil {
		a.id = &sc.UserID
	}
	if sc.Role == "" && sc.Privileged {
		a.role = systemActor
	}
	return a
}

// TransitionInput is what a caller says about a move. Version is the order
// version the caller saw.
type TransitionInput struct {
	Version int
	Reason  string
	// ExpectRevision, when set, must be the order's revision: a customer who
	// accepts a revision accepts that one and no other (T07).
	ExpectRevision *int
	// Revise is what the shop changed. Only T03 reads it.
	Revise *Revision
	// verified says the delivery code was checked. Only Complete sets it, and a
	// driver cannot make T16 without it.
	verified bool
}

// Transition is the only way an order changes status after it was submitted
// (docs/03 section 4, docs/07 section 9). Inside one transaction it locks the
// order, checks the scope, the version and the generated table for this actor,
// runs the side effects of the move, saves the new status, and writes the
// history, the outbox event and (for an admin) the audit row.
func (s *Service) Transition(ctx context.Context, scope db.Scope, orderID uuid.UUID, to enum.OrderStatus, in TransitionInput) (Detail, error) {
	var out Detail
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := s.transition(ctx, tx, scope, orderID, to, in); err != nil {
			return err
		}
		if _, shown := viewerOf(scope); !shown {
			return nil // the system acts without a screen: there is nothing to show it
		}
		var err error
		out, err = s.detail(ctx, tx, scope, orderID)
		return err
	})
	return out, err
}

// transition is Transition inside the caller's transaction, for callers that
// do more in the same one (a payment callback, a timer).
func (s *Service) transition(ctx context.Context, tx *db.Tx, scope db.Scope, orderID uuid.UUID, to enum.OrderStatus, in TransitionInput) error {
	o, found, err := lockOrder(ctx, tx, orderID)
	if err != nil || !found || !inScope(scope, o) {
		return apperr.NotFoundUnless(err, found && inScope(scope, o))
	}
	if in.Version != o.Version || (in.ExpectRevision != nil && *in.ExpectRevision != o.RevisionNo) {
		return apperr.New(apperr.StaleOrder)
	}
	who := actorOf(scope)
	t, ok := Lookup(o.Status, to)
	if !ok || !t.AllowedFor(who.role) {
		return apperr.New(apperr.InvalidTransition)
	}
	if t.ID == "T16" && who.role != string(enum.RoleSuperAdmin) && !in.verified {
		return apperr.New(apperr.InvalidTransition) // delivered needs the customer's code
	}
	if t.ReasonRequired && in.Reason == "" {
		return apperr.Validation(map[string]string{"reason": "required"})
	}
	if utf8.RuneCountInString(in.Reason) > maxReason {
		return apperr.Validation(map[string]string{"reason": "too_long"})
	}
	hook, built := hooks[t.ID]
	if !built {
		return errNotBuilt(t.ID)
	}
	if err := hook(ctx, s, tx, &o, in); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE orders SET status = $3::text, version = version + 1, status_reason = NULLIF($4, ''),
			decided_at  = CASE WHEN $3::text IN ('AWAITING_PAYMENT', 'REVISED', 'REJECTED') THEN $5 ELSE decided_at END,
			paid_at     = CASE WHEN $3::text = 'PAID' THEN $5 ELSE paid_at END,
			picking_at  = CASE WHEN $3::text = 'PICKING' THEN $5 ELSE picking_at END,
			packed_at   = CASE WHEN $3::text = 'PACKED' THEN $5 ELSE packed_at END,
			delivered_at = CASE WHEN $3::text = 'DELIVERED' THEN $5 ELSE delivered_at END,
			closed_at   = CASE WHEN $6 THEN $5 ELSE closed_at END
		WHERE id = $1 AND version = $2`, o.ID, o.Version, to, in.Reason, clock.Now(), IsFinal(to))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return apperr.New(apperr.StaleOrder)
	}
	from := o.Status
	if err := s.record(ctx, tx, o, &from, to, who, in.Reason); err != nil {
		return err
	}
	if who.role == string(enum.RoleSuperAdmin) {
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "order.transition", Entity: "order", EntityID: o.ID.String(), ShopID: o.ShopID,
			Before: map[string]any{"status": from}, After: map[string]any{"status": to, "reason": in.Reason}})
	}
	return nil
}

// record writes the history row and the outbox event of a move. T01 uses it
// too, with no "from".
func (s *Service) record(ctx context.Context, tx *db.Tx, o orderRow, from *enum.OrderStatus, to enum.OrderStatus, who actor, reason string) error {
	if err := writeEvent(ctx, tx, o, from, to, who, reason); err != nil {
		return err
	}
	_, err := events.Add(ctx, tx, events.Event{Type: eventFor(to), ShopID: o.ShopID, EntityID: o.ID.String(),
		Payload: map[string]any{"orderId": o.ID, "code": o.Code, "from": from, "to": to, "revisionNo": o.RevisionNo}})
	return err
}

// inScope is the second check after row level security: the caller must own
// the order, belong to its shop, or be its driver. An order that fails it is
// not found, never forbidden (CLAUDE.md rule 9).
func inScope(sc db.Scope, o orderRow) bool {
	switch {
	case sc.Privileged:
		return sc.ShopID == uuid.Nil || o.ShopID == sc.ShopID // an admin on a shop's console sees that shop only
	case sc.Role == enum.RoleCustomer:
		return o.CustomerID == sc.UserID
	case sc.Role == enum.RoleShopDriver || sc.Role == enum.RolePlatformDriver:
		return o.DriverID != nil && *o.DriverID == sc.UserID
	}
	return sc.ShopID != uuid.Nil && o.ShopID == sc.ShopID
}

func errNotBuilt(id string) error {
	return errors.New("order: transition " + id + " has no side-effect hook yet")
}
