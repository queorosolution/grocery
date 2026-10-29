package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shared/keyset"
	"gp/internal/shared/money"
)

const (
	defaultLimit = 20
	maxLimit     = 50
)

// Summary is one row of the customer's order list.
type Summary struct {
	ID         uuid.UUID        `json:"id"`
	Code       string           `json:"code"`
	Status     enum.OrderStatus `json:"status"`
	Shop       SummaryShop      `json:"shop"`
	TotalCents money.Cents      `json:"totalCents"`
	ItemCount  int              `json:"itemCount"`
	CreatedAt  time.Time        `json:"createdAt"`
}

type SummaryShop struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// SummaryPage is a page of the list; NextCursor is nil on the last page.
type SummaryPage struct {
	Data       []Summary `json:"data"`
	NextCursor *string   `json:"nextCursor"`
}

// listKey is where a page ended: the list runs newest first, and (time, id) is a stable order.
type listKey struct {
	At time.Time `json:"t"`
	ID uuid.UUID `json:"i"`
}

// List returns the caller's own orders, newest first. limit 0 means the default.
func (s *Service) List(ctx context.Context, scope db.Scope, limit int, cursor string) (SummaryPage, error) {
	if limit <= 0 {
		limit = defaultLimit
	}
	limit = min(limit, maxLimit)
	after, err := keyset.Decode(cursor, func(k listKey) uuid.UUID { return k.ID })
	if err != nil {
		return SummaryPage{}, err
	}
	var rows []Summary
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) (err error) {
		rows, err = summaries(ctx, tx, scope.UserID, after, limit+1)
		return err
	})
	if err != nil {
		return SummaryPage{}, err
	}
	page := SummaryPage{Data: rows}
	if len(rows) > limit {
		page.Data = rows[:limit]
		last := page.Data[limit-1]
		next := keyset.Encode(listKey{At: last.CreatedAt, ID: last.ID})
		page.NextCursor = &next
	}
	return page, nil
}

func summaries(ctx context.Context, tx *db.Tx, userID uuid.UUID, after *listKey, n int) ([]Summary, error) {
	var at *time.Time
	var id uuid.UUID
	if after != nil {
		at, id = &after.At, after.ID
	}
	rows, err := tx.Query(ctx, `
		SELECT o.id, o.code, o.status, sh.slug, sh.name, o.total_cents, o.created_at,
		       (SELECT count(*) FROM order_items i WHERE i.order_id = o.id AND i.revision_no = o.revision_no)
		FROM orders o JOIN shops sh ON sh.id = o.shop_id
		WHERE o.customer_id = $1 AND ($2::timestamptz IS NULL OR (o.created_at, o.id) < ($2, $3))
		ORDER BY o.created_at DESC, o.id DESC LIMIT $4`, userID, at, id, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Summary{}
	for rows.Next() {
		var r Summary
		if err := rows.Scan(&r.ID, &r.Code, &r.Status, &r.Shop.Slug, &r.Shop.Name, &r.TotalCents, &r.CreatedAt, &r.ItemCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
