package order

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/ids"
	"gp/internal/shared/money"
	"gp/internal/shared/validate"
)

// ConsoleSummary is one row of the shop's order list: enough to pick the order
// to open, with the phone masked as in the detail.
type ConsoleSummary struct {
	ID         uuid.UUID        `json:"id"`
	ShopID     uuid.UUID        `json:"shopId"`
	Code       string           `json:"code"`
	Status     enum.OrderStatus `json:"status"`
	Version    int              `json:"version"`
	Customer   CustomerRef      `json:"customer"`
	Slot       *SlotRef         `json:"slot"`
	TotalCents money.Cents      `json:"totalCents"`
	ItemCount  int              `json:"itemCount"`
	CreatedAt  time.Time        `json:"createdAt"`
}

// ConsolePage is a page of the shop's list.
type ConsolePage = Page[ConsoleSummary]

// maxStatusFilter is the length of the longest status list; there are only 13 statuses.
const maxStatusFilter = 13

// StatusFilter reads ?status=A,B. An empty value means every status. A status
// that does not exist is a bad request, and a repeated one is counted once.
func StatusFilter(raw string) ([]enum.OrderStatus, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	v := validate.New().Check("status", len(parts) <= maxStatusFilter, "invalid")
	var out []enum.OrderStatus
	seen := map[enum.OrderStatus]bool{}
	for _, p := range parts {
		st := enum.OrderStatus(p)
		v.Check("status", st.Valid(), "invalid")
		if !seen[st] {
			seen[st] = true
			out = append(out, st)
		}
	}
	return out, v.Err()
}

// ConsoleList is the shop's orders, newest first, optionally of some statuses.
// The shop is the one in the caller's scope; it never comes from the request.
func (s *Service) ConsoleList(ctx context.Context, scope db.Scope, statuses []enum.OrderStatus, limit int, cursor string) (ConsolePage, error) {
	limit, after, err := pageStart(limit, cursor)
	if err != nil {
		return ConsolePage{}, err
	}
	var rows []ConsoleSummary
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		rows, err = consoleRows(ctx, tx, consoleFilter{ShopID: scope.ShopID, Statuses: statuses}, after, limit+1)
		return err
	})
	if err != nil {
		return ConsolePage{}, err
	}
	return pageOf(rows, limit, func(r ConsoleSummary) listKey { return listKey{At: r.CreatedAt, ID: r.ID} }), nil
}

// consoleFilter narrows a list of orders. A Nil ShopID means every shop, which
// only the admin's scope can see.
type consoleFilter struct {
	ShopID   uuid.UUID
	Statuses []enum.OrderStatus
	Q        string
}

func consoleRows(ctx context.Context, tx *db.Tx, f consoleFilter, after *listKey, n int) ([]ConsoleSummary, error) {
	var at *time.Time
	var id uuid.UUID
	if after != nil {
		at, id = &after.At, after.ID
	}
	filter := make([]string, len(f.Statuses))
	for i, st := range f.Statuses {
		filter[i] = string(st)
	}
	rows, err := tx.Query(ctx, `
		SELECT o.id, o.shop_id, o.code, o.status, o.version, u.name, o.address->>'phone', sl.starts_at, sl.ends_at, o.total_cents, o.created_at,
		       (SELECT count(*) FROM order_items i WHERE i.order_id = o.id AND i.revision_no = o.revision_no AND i.state IN ('ACTIVE', 'SUBSTITUTE'))
		FROM orders o JOIN users u ON u.id = o.customer_id LEFT JOIN slots sl ON sl.id = o.slot_id
		WHERE ($1::uuid IS NULL OR o.shop_id = $1) AND (cardinality($2::text[]) = 0 OR o.status = ANY($2))
		  AND ($3::timestamptz IS NULL OR (o.created_at, o.id) < ($3, $4))
		  AND ($6 = '' OR o.code = upper($6) OR ($6 ~ '^[0-9]{4}$' AND right(o.address->>'phone', 4) = $6))
		ORDER BY o.created_at DESC, o.id DESC LIMIT $5`, ids.Ref(f.ShopID), filter, at, id, n, f.Q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConsoleSummary{}
	for rows.Next() {
		var r ConsoleSummary
		var phone string
		var starts, ends *time.Time
		if err := rows.Scan(&r.ID, &r.ShopID, &r.Code, &r.Status, &r.Version, &r.Customer.Name, &phone, &starts, &ends, &r.TotalCents, &r.CreatedAt, &r.ItemCount); err != nil {
			return nil, err
		}
		r.Customer.PhoneMasked = maskPhone(phone)
		if starts != nil && ends != nil {
			r.Slot = &SlotRef{StartsAt: *starts, EndsAt: *ends}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
