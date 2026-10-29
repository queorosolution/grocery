// Package apperr is the one error type services return. Codes come from the
// contract (codes_gen.go); the client only ever sees the code, its message and
// per-field reasons, never the cause.
package apperr

import "errors"

type Error struct {
	Code Code
	// Fields maps a request field to a short reason ("required", "too_long").
	// Reasons are fixed words, never user input.
	Fields map[string]string
	cause  error
}

func New(code Code) *Error { return &Error{Code: code} }

// Validation builds a VALIDATION_ERROR with per-field reasons.
func Validation(fields map[string]string) *Error {
	return &Error{Code: ValidationError, Fields: fields}
}

// Wrap keeps the real cause for logs and errors.Is, and never sends it to the client.
func (e *Error) Wrap(cause error) *Error {
	c := *e
	c.cause = cause
	return &c
}

func (e *Error) Error() string {
	if e.cause != nil {
		return string(e.Code) + ": " + e.cause.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error { return e.cause }

// From returns err as an *Error. Anything else becomes INTERNAL with the
// original error kept as the cause, so handlers have one path for every error.
func From(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return New(Internal).Wrap(err)
}

// Is reports whether err carries the given code.
func Is(err error, code Code) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// NotFoundUnless is for a lookup that reports "found" separately: it passes a
// database error through, answers NOT_FOUND for a missing row, and nil otherwise.
func NotFoundUnless(err error, found bool) error {
	if err != nil {
		return err
	}
	if !found {
		return New(NotFound)
	}
	return nil
}
