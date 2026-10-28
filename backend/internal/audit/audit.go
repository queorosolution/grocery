// Package audit is the append-only record of who changed what (docs/05 §3,
// docs/10 §6). Writing joins the caller's transaction, so a change and its
// audit row commit or fail together. Only the admin can read it back.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/logx"
)

// Entry is one change. Before and After are small maps or structs of the fields
// that changed; secret-looking keys are masked on the way in, but callers must
// still leave secrets out on purpose.
type Entry struct {
	Action   string // "shop.update"
	Entity   string // "shop"
	EntityID string
	ShopID   uuid.UUID // uuid.Nil when the change is not about one shop
	Before   any
	After    any
}

// Write adds the row inside tx. The actor comes from the scope the route guard
// built, never from the request.
func Write(ctx context.Context, tx *db.Tx, scope db.Scope, e Entry) error {
	before, err := encode(e.Before)
	if err != nil {
		return err
	}
	after, err := encode(e.After)
	if err != nil {
		return err
	}
	var actor, shop *uuid.UUID
	if scope.UserID != uuid.Nil {
		actor = &scope.UserID
	}
	if e.ShopID != uuid.Nil {
		shop = &e.ShopID
	}
	role := string(scope.Role)
	if role == "" {
		role = "SYSTEM"
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, actor_role, action, entity, entity_id, shop_id, before, after)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7::jsonb, $8::jsonb)`,
		actor, role, e.Action, e.Entity, e.EntityID, shop, before, after)
	return err
}

// encode returns JSON text with secret keys masked, or nil for "no value".
func encode(v any) (*string, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(logx.RedactSecrets(v))
	if err != nil {
		return nil, fmt.Errorf("audit: encode: %w", err)
	}
	s := string(raw)
	return &s, nil
}

// Row is one audit entry as the admin sees it.
type Row struct {
	ID        int64           `json:"id"`
	ActorID   *uuid.UUID      `json:"actorId"`
	ActorRole string          `json:"actorRole"`
	Action    string          `json:"action"`
	Entity    string          `json:"entity"`
	EntityID  *string         `json:"entityId"`
	ShopID    *uuid.UUID      `json:"shopId"`
	Before    json.RawMessage `json:"before"`
	After     json.RawMessage `json:"after"`
	At        time.Time       `json:"at"`
}

// Page is a slice of the log, newest first. NextCursor is empty on the last page.
type Page struct {
	Data       []Row  `json:"data"`
	NextCursor string `json:"nextCursor"`
}

// Query narrows the log. Cursor is the id of the last row of the previous page.
type Query struct {
	Entity   string
	EntityID string
	Cursor   int64
}

// PageSize is the rows per page.
const PageSize = 50

// List returns one page, newest first. Every filter is optional and the sort is
// on the primary key, so paging is stable while new rows arrive.
func List(ctx context.Context, tx *db.Tx, q Query) (Page, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, actor_id, actor_role, action, entity, entity_id, shop_id, before, after, at
		FROM audit_log
		WHERE ($1 = '' OR entity = $1)
		  AND ($2 = '' OR entity_id = $2)
		  AND ($3 = 0 OR id < $3)
		ORDER BY id DESC LIMIT $4`, q.Entity, q.EntityID, q.Cursor, PageSize+1)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{Data: []Row{}}
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.ActorID, &r.ActorRole, &r.Action, &r.Entity, &r.EntityID, &r.ShopID, &r.Before, &r.After, &r.At); err != nil {
			return Page{}, err
		}
		page.Data = append(page.Data, r)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Data) > PageSize {
		page.Data = page.Data[:PageSize]
		page.NextCursor = fmt.Sprint(page.Data[PageSize-1].ID)
	}
	return page, nil
}

// ParseCursor reads the cursor from a query string; empty means the first page.
func ParseCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n <= 0 || fmt.Sprint(n) != s {
		return 0, apperr.Validation(map[string]string{"cursor": "invalid"})
	}
	return n, nil
}
