package refund

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

const maxReason = 300

func conflict(field, reason string) error {
	return &apperr.Error{Code: apperr.Conflict, Fields: map[string]string{field: reason}}
}

// Request is the shop's ask (FR-RFD-02): a full cancel while the order is paid or
// being picked, or one missing line while it is being picked. The line is marked
// missing at once, so it cannot go out. Only one request can hold the customer's
// money at a time: a full one excludes any other, and a line is asked for once.
func (s *Service) Request(ctx context.Context, scope db.Scope, orderID uuid.UUID, in RequestInput) (Refund, error) {
	v := validate.New().Text("reason", in.Reason, 3, maxReason).
		Check("type", in.Type == enum.RefundTypeFull || in.Type == enum.RefundTypePartial, "invalid")
	v.Check("orderItemId", (in.Type == enum.RefundTypePartial) == (in.OrderItemID != nil), "invalid")
	if err := v.Err(); err != nil {
		return Refund{}, err
	}
	var out Refund
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		o, err := s.orders.RefundOrder(ctx, tx, scope, orderID)
		if err != nil {
			return err
		}
		allowed := []enum.OrderStatus{enum.OrderStatusPicking}
		if in.Type == enum.RefundTypeFull {
			allowed = append(allowed, enum.OrderStatusPaid)
		}
		if !slices.Contains(allowed, o.Status) {
			return apperr.New(apperr.InvalidTransition)
		}
		n, full, _, _, err := openFor(ctx, tx, orderID, uuid.Nil)
		switch {
		case err != nil:
			return err
		case in.Type == enum.RefundTypeFull && n > 0, full:
			return conflict("refund", "exists")
		}
		req := newRefund{OrderID: orderID, ShopID: o.ShopID, Type: in.Type, ItemID: in.OrderItemID, AmountCents: o.TotalCents,
			Reason: in.Reason, Status: enum.RefundStatusRequested, RequestedBy: scope.UserID}
		if in.Type == enum.RefundTypePartial {
			line, err := s.orders.MarkLineMissing(ctx, tx, scope, orderID, *in.OrderItemID)
			if err != nil {
				return err
			}
			req.AmountCents = line.LineCents
		}
		id, err := insert(ctx, tx, req)
		if err != nil {
			return err
		}
		if err := announce(ctx, tx, id, req); err != nil {
			return err
		}
		if err := audit.Write(ctx, tx, scope, audit.Entry{Action: "refund.request", Entity: "refund", EntityID: id.String(), ShopID: o.ShopID,
			After: map[string]any{"type": in.Type, "amountCents": req.AmountCents}}); err != nil {
			return err
		}
		out, _, err = read(ctx, tx, id)
		return err
	})
	return out, err
}

func read(ctx context.Context, tx *db.Tx, id uuid.UUID) (Refund, bool, error) {
	r, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM refunds r JOIN orders o ON o.id = r.order_id WHERE r.id = $1`, id))
	return r, err == nil, db.IgnoreNoRows(err)
}

// List is the admin's queue, newest first. status is empty for all.
func (s *Service) List(ctx context.Context, scope db.Scope, status string) ([]Refund, error) {
	if err := validate.New().Check("status", status == "" || enum.RefundStatus(status).Valid(), "invalid").Err(); err != nil {
		return nil, err
	}
	out := []Refund{}
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+columns+` FROM refunds r JOIN orders o ON o.id = r.order_id
			WHERE ($1 = '' OR r.status = $1) ORDER BY r.created_at DESC, r.id LIMIT $2`, status, listLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scan(rows)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
