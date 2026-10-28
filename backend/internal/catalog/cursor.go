package catalog

import (
	"encoding/base64"
	"encoding/json"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
)

// pageSize is the rows per page of the admin and console lists.
const pageSize = 50

// cursor is where the previous page ended. Lists are ordered by name, then id,
// so the pair is a stable keyset.
type cursor struct {
	Name string    `json:"n"`
	ID   uuid.UUID `json:"i"`
}

func (c cursor) String() string {
	raw, _ := json.Marshal(c) // two plain fields cannot fail to marshal
	return base64.RawURLEncoding.EncodeToString(raw)
}

// parseCursor reads a cursor from a query string; empty means the first page.
func parseCursor(s string) (*cursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	var c cursor
	if err != nil || json.Unmarshal(raw, &c) != nil || c.ID == uuid.Nil {
		return nil, apperr.Validation(map[string]string{"cursor": "invalid"})
	}
	return &c, nil
}

// cut turns pageSize+1 fetched rows into a page and its next cursor.
func cut[T any](rows []T, key func(T) cursor) Page[T] {
	if len(rows) <= pageSize {
		return Page[T]{Data: nonNil(rows)}
	}
	rows = rows[:pageSize]
	return Page[T]{Data: rows, NextCursor: key(rows[pageSize-1]).String()}
}

func nonNil[T any](rows []T) []T {
	if rows == nil {
		return []T{}
	}
	return rows
}
