package delivery

import (
	"bytes"
	"fmt"
	"strconv"
)

// Dec2 is a non-negative number with two decimals, kept as hundredths (2.6 km
// is 260). Distances, weights and the road factor use it so the fee is exact
// integer maths and never touches a float.
type Dec2 int64

func (d Dec2) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("%d.%02d", int64(d)/100, int64(d)%100)), nil
}

// UnmarshalJSON accepts a plain JSON number with at most two decimals ("2",
// "2.5", "2.50"). Strings, exponents and negatives are refused.
func (d *Dec2) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	whole, frac, _ := bytes.Cut(b, []byte("."))
	if len(whole) == 0 || len(whole) > 6 || len(frac) > 2 || !digits(whole) || !digits(frac) {
		return fmt.Errorf("delivery: %q is not a number with up to 2 decimals", b)
	}
	n, err := strconv.ParseInt(string(whole), 10, 64)
	if err != nil {
		return err
	}
	f := int64(0)
	if len(frac) > 0 {
		f, _ = strconv.ParseInt(string(frac), 10, 64)
		if len(frac) == 1 {
			f *= 10
		}
	}
	*d = Dec2(n*100 + f)
	return nil
}

func digits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
