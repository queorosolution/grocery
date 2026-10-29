// Package httpx is the HTTP plumbing every handler uses: strict JSON in, one
// error shape out, security headers, and the middleware chain (docs/07 section 3).
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"gp/internal/shared/apperr"
)

// MaxBody is the largest request body, 64 KB (docs/06 section 1).
const MaxBody = 64 << 10

// MaxWebhookBody is the limit for signed server-to-server calls.
const MaxWebhookBody = 16 << 10

// Decode reads one JSON object into dst. Unknown fields, trailing data, a wrong
// content type and bodies over 64 KB are all rejected. Error reasons are fixed
// words: nothing the client sent is echoed back.
func Decode(r *http.Request, dst any) error {
	if r.ContentLength != 0 && r.Header.Get("Content-Type") != "" {
		if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
			return apperr.Validation(map[string]string{"contentType": "invalid"})
		}
	}
	// The middleware already limits the body; this keeps Decode safe on its own.
	body := http.MaxBytesReader(nil, r.Body, MaxBody)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	// A raw value, so extra JSON is "trailing data" and not an unknown field.
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			err = errors.New("trailing data")
		}
		return decodeError(err)
	}
	return nil
}

func decodeError(err error) error {
	var tooBig *http.MaxBytesError
	var typeErr *json.UnmarshalTypeError
	switch {
	case errors.As(err, &tooBig):
		return apperr.New(apperr.PayloadTooLarge)
	case errors.Is(err, io.EOF):
		return apperr.Validation(map[string]string{"body": "required"})
	case errors.As(err, &typeErr) && typeErr.Field != "":
		// Field is our own struct path, not client input.
		return apperr.Validation(map[string]string{typeErr.Field: "invalid_type"})
	case strings.HasPrefix(err.Error(), "json: unknown field"):
		return apperr.Validation(map[string]string{"body": "unknown_field"})
	default:
		return apperr.Validation(map[string]string{"body": "invalid"})
	}
}

// PageLimit reads ?limit=: def when it is left out, VALIDATION_ERROR when it is
// not a number. The range is the list's own business.
func PageLimit(raw string, def int) (int, error) {
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperr.Validation(map[string]string{"limit": "invalid"})
	}
	return n, nil
}
