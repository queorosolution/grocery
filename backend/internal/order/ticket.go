package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
	"gp/internal/shared/ratelimit"
	"gp/internal/shared/validate"
)

// Support tickets are the customer's "report a problem" on a paid order
// (FR-RFD-05). They go to the admin queue; nothing else reads them.

// TicketReason is not in contracts/enums.json yet, so it is a code constant.
type TicketReason string

const (
	ReasonMissingItem TicketReason = "MISSING_ITEM"
	ReasonDamaged     TicketReason = "DAMAGED"
	ReasonWrongItem   TicketReason = "WRONG_ITEM"
	ReasonLate        TicketReason = "LATE"
	ReasonOther       TicketReason = "OTHER"
)

func (r TicketReason) valid() bool {
	switch r {
	case ReasonMissingItem, ReasonDamaged, ReasonWrongItem, ReasonLate, ReasonOther:
		return true
	}
	return false
}

const (
	maxTicketNote    = 500
	ticketsPerDay    = 10
	ticketListLimit  = 100
	ticketOpen       = "OPEN"
	ticketResolved   = "RESOLVED"
	ticketColumns    = `t.id, t.order_id, o.code, t.shop_id, t.reason, t.note, t.status, t.resolution, t.created_at, t.resolved_at`
	ticketFromOrders = ` FROM support_tickets t JOIN orders o ON o.id = t.order_id`
)

type Ticket struct {
	ID         uuid.UUID    `json:"id"`
	OrderID    uuid.UUID    `json:"orderId"`
	OrderCode  string       `json:"orderCode"`
	ShopID     uuid.UUID    `json:"shopId"`
	Reason     TicketReason `json:"reason"`
	Note       *string      `json:"note"`
	Status     string       `json:"status"`
	Resolution *string      `json:"resolution"`
	CreatedAt  time.Time    `json:"createdAt"`
	ResolvedAt *time.Time   `json:"resolvedAt"`
}

func scanTicket(row interface{ Scan(...any) error }) (t Ticket, err error) {
	err = row.Scan(&t.ID, &t.OrderID, &t.OrderCode, &t.ShopID, &t.Reason, &t.Note, &t.Status, &t.Resolution, &t.CreatedAt, &t.ResolvedAt)
	return t, err
}

// TicketInput is the body of POST /orders/{id}/tickets.
type TicketInput struct {
	Reason TicketReason `json:"reason"`
	Note   string       `json:"note"`
}

// ReportProblem opens a ticket on the customer's own paid order. The user and
// the shop come from the order and the scope, never from the request.
func (s *Service) ReportProblem(ctx context.Context, scope db.Scope, orderID uuid.UUID, in TicketInput) (Ticket, error) {
	if err := validate.New().Check("reason", in.Reason.valid(), "invalid").Text("note", in.Note, 0, maxTicketNote).Err(); err != nil {
		return Ticket{}, err
	}
	if err := ratelimit.Hit(ctx, s.db, "tickets:"+scope.UserID.String(), ticketsPerDay, 24*time.Hour); err != nil {
		return Ticket{}, err
	}
	var out Ticket
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		var shopID uuid.UUID
		var paid bool
		err := tx.QueryRow(ctx, `SELECT shop_id, paid_at IS NOT NULL FROM orders WHERE id = $1 AND customer_id = $2`, orderID, scope.UserID).Scan(&shopID, &paid)
		if err != nil {
			return apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
		}
		if !paid {
			return apperr.New(apperr.InvalidTransition) // nothing to report before the customer paid
		}
		out, err = scanTicket(tx.QueryRow(ctx, `
			WITH t AS (INSERT INTO support_tickets (order_id, shop_id, user_id, reason, note) VALUES ($1, $2, $3, $4, NULLIF($5, '')) RETURNING *)
			SELECT `+ticketColumns+` FROM t JOIN orders o ON o.id = t.order_id`, orderID, shopID, scope.UserID, in.Reason, in.Note))
		return err
	})
	return out, err
}

// Tickets is the admin's queue, oldest first so nothing waits at the bottom.
// status is OPEN or RESOLVED; empty means both.
func (s *Service) Tickets(ctx context.Context, scope db.Scope, status string) ([]Ticket, error) {
	if err := validate.New().Check("status", status == "" || status == ticketOpen || status == ticketResolved, "invalid").Err(); err != nil {
		return nil, err
	}
	out := []Ticket{}
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+ticketColumns+ticketFromOrders+` WHERE ($1 = '' OR t.status = $1)
			ORDER BY t.created_at, t.id LIMIT $2`, status, ticketListLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			t, err := scanTicket(rows)
			if err != nil {
				return err
			}
			out = append(out, t)
		}
		return rows.Err()
	})
	return out, err
}

// ResolveInput is the body of POST /admin/tickets/{id}/resolve.
type ResolveInput struct {
	Resolution string `json:"resolution"`
}

// ResolveTicket closes an open ticket with what was done about it.
func (s *Service) ResolveTicket(ctx context.Context, scope db.Scope, id uuid.UUID, in ResolveInput) (Ticket, error) {
	if err := validate.New().Text("resolution", in.Resolution, 3, maxTicketNote).Err(); err != nil {
		return Ticket{}, err
	}
	var out Ticket
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		var status string
		var shopID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT status, shop_id FROM support_tickets WHERE id = $1 FOR UPDATE`, id).Scan(&status, &shopID); err != nil {
			return apperr.NotFoundUnless(db.IgnoreNoRows(err), err == nil)
		}
		if status != ticketOpen {
			return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{"status": "not_open"}}
		}
		var err error
		out, err = scanTicket(tx.QueryRow(ctx, `
			WITH t AS (UPDATE support_tickets SET status = 'RESOLVED', resolution = $2, resolved_at = $3 WHERE id = $1 RETURNING *)
			SELECT `+ticketColumns+` FROM t JOIN orders o ON o.id = t.order_id`, id, in.Resolution, clock.Now()))
		if err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "ticket.resolve", Entity: "ticket", EntityID: id.String(), ShopID: shopID})
	})
	return out, err
}
