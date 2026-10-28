// Package money holds the only money math. Amounts are int64 cents
// (Rs 450.50 = 45050). No floats, and every multiply is overflow-checked.
package money

import (
	"errors"
	"math"
	"math/bits"
	"strconv"
	"strings"
)

type Cents int64

const rupee Cents = 100

var ErrOverflow = errors.New("money: amount overflows int64")

// RoundRupee rounds half away from zero to the nearest whole rupee (100 cents).
// For prices, which are never negative, that is "half up" (docs/07 §6).
func RoundRupee(c Cents) (Cents, error) {
	half := rupee / 2
	switch {
	case c >= 0 && c > math.MaxInt64-half, c < 0 && c < math.MinInt64+half:
		return 0, ErrOverflow
	case c >= 0:
		return (c + half) / rupee * rupee, nil
	default:
		return -((-c + half) / rupee * rupee), nil
	}
}

// ApplyBP returns c × bp / 10000 rounded half away from zero. 250 bp is 2.5%.
func ApplyBP(c Cents, bp int) (Cents, error) {
	return mulDivRound(int64(c), int64(bp), 10000)
}

// MulQty returns unit price × whole-number quantity (D28).
func MulQty(unit Cents, qty int64) (Cents, error) {
	return mulDivRound(int64(unit), qty, 1)
}

// Add returns a + b, or ErrOverflow.
func Add(a, b Cents) (Cents, error) {
	sum := a + b
	if (a > 0 && b > 0 && sum < 0) || (a < 0 && b < 0 && sum >= 0) {
		return 0, ErrOverflow
	}
	return sum, nil
}

// mulDivRound computes a×b/d rounded half away from zero using a 128-bit
// intermediate, so a large a×b never wraps silently.
func mulDivRound(a, b, d int64) (Cents, error) {
	neg := (a < 0) != (b < 0)
	ua, ub := abs(a), abs(b)
	ud := uint64(d) // #nosec G115 -- d is always a positive constant divisor (1 or 10000)
	hi, lo := bits.Mul64(ua, ub)
	if hi >= ud {
		return 0, ErrOverflow
	}
	q, r := bits.Div64(hi, lo, ud)
	if r >= ud-r { // remainder is at least half
		q++
	}
	if q > math.MaxInt64 {
		return 0, ErrOverflow
	}
	if neg {
		return Cents(-int64(q)), nil
	}
	return Cents(q), nil
}

func abs(v int64) uint64 {
	if v < 0 {
		return uint64(-v) // #nosec G115 -- also right for MinInt64: -MinInt64 wraps to 2^63 as uint64
	}
	return uint64(v)
}

// FormatLKR renders cents for people: 143500 -> "Rs 1,435.00".
func FormatLKR(c Cents) string {
	sign := ""
	u := abs(int64(c))
	if c < 0 {
		sign = "-"
	}
	whole := strconv.FormatUint(u/uint64(rupee), 10)
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	frac := strconv.FormatUint(u%uint64(rupee), 10)
	if len(frac) < 2 {
		frac = "0" + frac
	}
	return sign + "Rs " + b.String() + "." + frac
}
