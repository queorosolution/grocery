package connector

import (
	"strings"

	"gp/internal/pricing"
	"gp/internal/shared/enum"
)

const maxShopPrice = pricing.MaxShopPriceCents

// digits reads a non-empty run of ASCII digits, at most 15 of them.
func digits(s string) (int64, bool) {
	if s == "" || len(s) > 15 {
		return 0, false
	}
	var n int64
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int64(s[i]-'0')
	}
	return n, true
}

// ParseMoney reads a decimal text such as "1250", "1250.5" or "12.345" into
// cents, without floating point. In RUPEES the value is multiplied by 100, in
// CENTS it is already cents. Extra decimals are rounded half up. Signs,
// exponents, thousands separators and text are refused: a price the source
// wrote in a way we do not understand is held, not guessed.
func ParseMoney(s string, unit enum.MoneyUnit) (int64, bool) {
	s = strings.TrimSpace(s)
	whole, frac, _ := strings.Cut(s, ".")
	if s == "" || strings.HasSuffix(s, ".") {
		return 0, false
	}
	w, ok := digits(whole)
	if !ok {
		return 0, false
	}
	if len(frac) > 15 || (frac != "" && !allDigits(frac)) {
		return 0, false
	}
	keep := 2 // decimals that survive: cents for rupees
	if unit == enum.MoneyUnitCents {
		keep = 0
	}
	cents := w
	if unit == enum.MoneyUnitRupees {
		cents = w * 100
	}
	cents += fracPart(frac, keep)
	return cents, true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// fracPart returns the first keep decimals as a number, rounded half up using the next one.
func fracPart(frac string, keep int) int64 {
	var n int64
	for i := 0; i < keep; i++ {
		n *= 10
		if i < len(frac) {
			n += int64(frac[i] - '0')
		}
	}
	if len(frac) > keep && frac[keep] >= '5' {
		n++
	}
	return n
}
