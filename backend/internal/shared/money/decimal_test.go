package money

import "testing"

func TestDecimal(t *testing.T) {
	for c, want := range map[Cents]string{0: "0.00", 5: "0.05", 100: "1.00", 158500: "1585.00", 45050: "450.50", -1: ""} {
		if got := Decimal(c); got != want {
			t.Errorf("Decimal(%d) = %q, want %q", c, got, want)
		}
	}
}

func TestParseDecimal(t *testing.T) {
	for in, want := range map[string]Cents{"0.00": 0, "0.05": 5, "1585.00": 158500, "450.50": 45050} {
		if got, ok := ParseDecimal(in); !ok || got != want {
			t.Errorf("ParseDecimal(%q) = %d, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "1585", "1585.0", "1585.000", "1,585.00", "-1.00", "+1.00", ".50", "1.5a", " 1.00", "99999999999999999.00"} {
		if _, ok := ParseDecimal(in); ok {
			t.Errorf("ParseDecimal(%q) was accepted", in)
		}
	}
	for c := Cents(0); c < 3000; c += 7 {
		if back, ok := ParseDecimal(Decimal(c)); !ok || back != c {
			t.Errorf("round trip of %d gave %d, %v", c, back, ok)
		}
	}
}
