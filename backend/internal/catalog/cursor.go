package catalog

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
)

// pageSize is the rows per page of the admin and console lists.
const pageSize = 50

// cursor is where the previous page of a list ordered by name ended. The pair
// (name, id) is a stable keyset.
type cursor struct {
	Name string    `json:"n"`
	ID   uuid.UUID `json:"i"`
}

// priceCursor is where a search page ended. Search lists available products
// first, then cheapest first, so the keyset is those three values.
type priceCursor struct {
	Unavailable bool      `json:"u"`
	Price       int64     `json:"p"`
	ID          uuid.UUID `json:"i"`
}

func (c cursor) String() string      { return encodeCursor(c) }
func (c priceCursor) String() string { return encodeCursor(c) }

func encodeCursor(v any) string {
	raw, _ := json.Marshal(v) // plain fields cannot fail to marshal
	return base64.RawURLEncoding.EncodeToString(raw)
}

// parseCursor reads a cursor from a query string; empty means the first page.
func parseCursor(s string) (*cursor, error) {
	return decodeCursor(s, func(c cursor) uuid.UUID { return c.ID })
}

func parsePriceCursor(s string) (*priceCursor, error) {
	return decodeCursor(s, func(c priceCursor) uuid.UUID { return c.ID })
}

func decodeCursor[T any](s string, id func(T) uuid.UUID) (*T, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	var c T
	if err != nil || json.Unmarshal(raw, &c) != nil || id(c) == uuid.Nil {
		return nil, apperr.Validation(map[string]string{"cursor": "invalid"})
	}
	return &c, nil
}

// split cuts the extra row off a page fetched with limit+1 rows. next is the
// cursor of the last kept row, or "" on the last page.
func split[T any](rows []T, limit int, key func(T) fmt.Stringer) (page []T, next string) {
	if len(rows) <= limit {
		return nonNil(rows), ""
	}
	rows = rows[:limit]
	return rows, key(rows[limit-1]).String()
}

// cut is split for the admin and console lists: an empty cursor ends the list.
func cut[T any](rows []T, key func(T) fmt.Stringer) Page[T] {
	data, next := split(rows, pageSize, key)
	return Page[T]{Data: data, NextCursor: next}
}

// publicCut is split for the public lists: a null cursor ends the list.
func publicCut[T any](rows []T, limit int, key func(T) fmt.Stringer) PublicPage[T] {
	data, next := split(rows, limit, key)
	if next == "" {
		return PublicPage[T]{Data: data}
	}
	return PublicPage[T]{Data: data, NextCursor: &next}
}

func nonNil[T any](rows []T) []T {
	if rows == nil {
		return []T{}
	}
	return rows
}
