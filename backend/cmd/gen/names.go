package main

import (
	"strings"
	"unicode"
)

// pascal turns SUPER_ADMIN, order.status_changed or PaymentStatus into
// SuperAdmin, OrderStatusChanged and PaymentStatus.
func pascal(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	var b strings.Builder
	for _, p := range parts {
		if p == strings.ToUpper(p) {
			p = strings.ToLower(p)
		}
		b.WriteString(strings.ToUpper(p[:1]) + p[1:])
	}
	return b.String()
}

func lowerFirst(s string) string {
	return strings.ToLower(s[:1]) + s[1:]
}
