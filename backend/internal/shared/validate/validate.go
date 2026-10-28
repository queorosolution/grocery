// Package validate checks request fields and returns one VALIDATION_ERROR with a
// reason per field. Reasons are fixed words; user input is never echoed back.
package validate

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
)

type Validator struct{ fields map[string]string }

func New() *Validator { return &Validator{fields: map[string]string{}} }

// fail keeps the first reason per field, which is the most useful one.
func (v *Validator) fail(field, reason string) *Validator {
	if _, ok := v.fields[field]; !ok {
		v.fields[field] = reason
	}
	return v
}

// Err returns nil when every check passed.
func (v *Validator) Err() error {
	if len(v.fields) == 0 {
		return nil
	}
	return apperr.Validation(v.fields)
}

// Str checks a single-line text of min..max characters (runes, after trimming).
func (v *Validator) Str(field, s string, min, max int) *Validator {
	return v.text(field, s, min, max, false)
}

// Text is Str for notes and addresses: new lines and tabs are allowed.
func (v *Validator) Text(field, s string, min, max int) *Validator {
	return v.text(field, s, min, max, true)
}

func (v *Validator) text(field, s string, min, max int, multiline bool) *Validator {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	switch {
	case !utf8.ValidString(s):
		return v.fail(field, "invalid")
	case n == 0 && min > 0:
		return v.fail(field, "required")
	case n < min:
		return v.fail(field, "too_short")
	case n > max:
		return v.fail(field, "too_long")
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return v.fail(field, "invalid")
		}
	}
	return v
}

// Int checks min <= n <= max.
func (v *Validator) Int(field string, n, min, max int64) *Validator {
	if n < min || n > max {
		return v.fail(field, "out_of_range")
	}
	return v
}

// Phone accepts the forms a Sri Lankan customer types; see NormalizePhone.
func (v *Validator) Phone(field, s string) *Validator {
	if _, ok := NormalizePhone(s); !ok {
		return v.fail(field, "invalid")
	}
	return v
}

// Email accepts a bare address only: no display name, no angle brackets.
func (v *Validator) Email(field, s string) *Validator {
	s = strings.TrimSpace(s)
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || len(s) > 254 {
		return v.fail(field, "invalid")
	}
	return v
}

// UUID checks the canonical 36-character form.
func (v *Validator) UUID(field, s string) *Validator {
	if _, err := uuid.Parse(s); err != nil || len(s) != 36 {
		return v.fail(field, "invalid")
	}
	return v
}

// Check records a failure when ok is false. Use it for enum checks:
// v.Check("status", status.Valid(), "invalid").
func (v *Validator) Check(field string, ok bool, reason string) *Validator {
	if !ok {
		return v.fail(field, reason)
	}
	return v
}

// NormalizePhone turns 0771234567, 94771234567 or +94 77 123 4567 into
// +94771234567. Only Sri Lankan numbers (D24: +94 only) are accepted.
func NormalizePhone(s string) (string, bool) {
	var digits strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '+' && i == 0, r == ' ', r == '-':
		default:
			return "", false
		}
	}
	d := digits.String()
	switch {
	case strings.HasPrefix(d, "94") && len(d) == 11:
		d = d[2:]
	case strings.HasPrefix(d, "0") && len(d) == 10:
		d = d[1:]
	default:
		return "", false
	}
	if d[0] == '0' {
		return "", false
	}
	return "+94" + d, true
}
