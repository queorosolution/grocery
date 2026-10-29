package money

import (
	"strconv"
	"strings"
)

// Decimal writes cents the way payment gateways want them: no sign, no
// thousands separator, two decimals. 158500 -> "1585.00". Negative amounts have
// no place in a charge, so they give "".
func Decimal(c Cents) string {
	if c < 0 {
		return ""
	}
	frac := strconv.FormatInt(int64(c%rupee), 10)
	return strconv.FormatInt(int64(c/rupee), 10) + "." + strings.Repeat("0", 2-len(frac)) + frac
}

// ParseDecimal is the inverse of Decimal and as strict: digits, a dot, exactly
// two decimals. "1585", "1585.0", "1,585.00" and "-1.00" are all refused, so a
// gateway amount that is not in our exact form never matches ours.
func ParseDecimal(s string) (Cents, bool) {
	whole, frac, ok := strings.Cut(s, ".")
	if !ok || len(frac) != 2 || whole == "" || len(whole) > 15 || !digits(whole) || !digits(frac) {
		return 0, false
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, false
	}
	f, _ := strconv.ParseInt(frac, 10, 64)
	return Cents(w)*rupee + Cents(f), true
}

func digits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
