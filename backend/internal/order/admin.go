package order

import (
	"context"
	"math"
	"regexp"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// The admin's tools for orders of every shop (docs/06 section 10).

// searchRe is what ?q= may hold: an order code (letters, digits, dash) or the
// last four digits of the phone. Anything else is a bad request, not a search.
var searchRe = regexp.MustCompile(`^[A-Za-z0-9-]{3,16}$`)

// AdminList is the admin's order list: newest first, of one shop or all, of some
// statuses, optionally found by order code or phone last 4.
func (s *Service) AdminList(ctx context.Context, scope db.Scope, shopID uuid.UUID, statuses []enum.OrderStatus, q string, limit int, cursor string) (ConsolePage, error) {
	if err := validate.New().Check("q", q == "" || searchRe.MatchString(q), "invalid").Err(); err != nil {
		return ConsolePage{}, err
	}
	limit, after, err := pageStart(limit, cursor)
	if err != nil {
		return ConsolePage{}, err
	}
	var rows []ConsoleSummary
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		rows, err = consoleRows(ctx, tx, consoleFilter{ShopID: shopID, Statuses: statuses, Q: q}, after, limit+1)
		return err
	})
	if err != nil {
		return ConsolePage{}, err
	}
	return pageOf(rows, limit, func(r ConsoleSummary) listKey { return listKey{At: r.CreatedAt, ID: r.ID} }), nil
}

// AdminMoveInput is the body of POST /admin/orders/{id}/transition.
type AdminMoveInput struct {
	Version int              `json:"version"`
	To      enum.OrderStatus `json:"to"`
	Reason  string           `json:"reason"`
}

// AdminTransition is the admin's override: any move the table allows the admin,
// with a reason (FR-ADM-03). The table, the version and the audit row are
// Transition's. Payment is the exception: T10 needs the payment record, so it only
// goes through mark-paid.
func (s *Service) AdminTransition(ctx context.Context, scope db.Scope, orderID uuid.UUID, in AdminMoveInput) (Detail, error) {
	v := validate.New().Int("version", int64(in.Version), 1, math.MaxInt32).Check("to", in.To.Valid(), "invalid").
		Check("reason", in.Reason != "", "required")
	v.Check("to", in.To != enum.OrderStatusPaid, "use_mark_paid")
	if err := v.Err(); err != nil {
		return Detail{}, err
	}
	return s.Transition(ctx, scope, orderID, in.To, TransitionInput{Version: in.Version, Reason: in.Reason})
}
