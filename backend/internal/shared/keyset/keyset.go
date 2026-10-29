// Package keyset is the opaque "where the last page ended" value of a list
// (docs/06 section 1). Each list defines its own small key struct; this package
// only turns it into text and back.
package keyset

import (
	"encoding/base64"
	"encoding/json"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
)

// Encode returns the key as URL-safe text.
func Encode(key any) string {
	raw, _ := json.Marshal(key) // plain fields cannot fail to marshal
	return base64.RawURLEncoding.EncodeToString(raw)
}

// Decode reads a key from a query string. Empty means the first page (nil, nil).
// A key with no id is refused, so a hand-made cursor cannot pass.
func Decode[T any](s string, id func(T) uuid.UUID) (*T, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	var k T
	if err != nil || json.Unmarshal(raw, &k) != nil || id(k) == uuid.Nil {
		return nil, apperr.Validation(map[string]string{"cursor": "invalid"})
	}
	return &k, nil
}
