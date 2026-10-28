// Package optional lets a PATCH body tell a field that was left out from a
// field sent as null. A plain pointer cannot: both decode to nil.
package optional

import (
	"bytes"
	"encoding/json"
)

// Field is one JSON field of a PATCH body. The zero value means "left out".
type Field[T any] struct {
	Set   bool // the field was in the body
	Null  bool // the body said null
	Value T
}

// UnmarshalJSON is called only when the field is present in the body.
func (f *Field[T]) UnmarshalJSON(b []byte) error {
	f.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		f.Null = true
		return nil
	}
	return json.Unmarshal(b, &f.Value)
}

// Ptr is the value when the field was sent with one, else nil.
func (f Field[T]) Ptr() *T {
	if !f.Set || f.Null {
		return nil
	}
	v := f.Value
	return &v
}
