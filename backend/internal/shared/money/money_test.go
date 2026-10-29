package money

import (
	"errors"
	"math"
	"testing"
)

func TestRoundRupeeHalfUp(t *testing.T) {
	for in, want := range map[Cents]Cents{0: 0, 49: 0, 50: 100, 99: 100, 100: 100, 149: 100, 150: 200, 143500: 143500, -49: 0, -50: -100, -150: -200} {
		got, err := RoundRupee(in)
		if err != nil || got != want {
			t.Errorf("RoundRupee(%d) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := RoundRupee(math.MaxInt64); !errors.Is(err, ErrOverflow) {
		t.Error("RoundRupee(MaxInt64) must overflow")
	}
}

// The worked example in docs/07 §6: Rs 1,400.00 at 250 bp is Rs 1,435.00.
func TestApplyBPMarkupExample(t *testing.T) {
	margin, err := ApplyBP(140000, 250)
	if err != nil || margin != 3500 {
		t.Fatalf("margin = %d, %v", margin, err)
	}
}

func TestApplyBPRounding(t *testing.T) {
	cases := []struct {
		c    Cents
		bp   int
		want Cents
	}{
		{1, 5000, 1},   // 0.5 rounds up
		{1, 4999, 0},   // 0.4999 rounds down
		{3, 5000, 2},   // 1.5 rounds up
		{-1, 5000, -1}, // half away from zero
		{100, 0, 0},
		{12345, 10000, 12345},
	}
	for _, tc := range cases {
		got, err := ApplyBP(tc.c, tc.bp)
		if err != nil || got != tc.want {
			t.Errorf("ApplyBP(%d, %d) = %d, %v; want %d", tc.c, tc.bp, got, err, tc.want)
		}
	}
}

func TestMulQty(t *testing.T) {
	got, err := MulQty(45050, 3)
	if err != nil || got != 135150 {
		t.Fatalf("got %d, %v", got, err)
	}
	if _, err := MulQty(math.MaxInt64, 2); !errors.Is(err, ErrOverflow) {
		t.Error("expected overflow")
	}
	if _, err := MulQty(math.MinInt64, 1); !errors.Is(err, ErrOverflow) {
		t.Error("MinInt64 has no positive twin, expected overflow")
	}
}

func TestAddOverflow(t *testing.T) {
	if got, err := Add(100, 250); err != nil || got != 350 {
		t.Fatalf("got %d, %v", got, err)
	}
	if _, err := Add(math.MaxInt64, 1); !errors.Is(err, ErrOverflow) {
		t.Error("expected overflow")
	}
	if _, err := Add(math.MinInt64, -1); !errors.Is(err, ErrOverflow) {
		t.Error("expected negative overflow")
	}
}

func TestFormatLKR(t *testing.T) {
	for in, want := range map[Cents]string{
		0: "Rs 0.00", 5: "Rs 0.05", 45050: "Rs 450.50", 143500: "Rs 1,435.00",
		123456789: "Rs 1,234,567.89", -5000: "-Rs 50.00",
	} {
		if got := FormatLKR(in); got != want {
			t.Errorf("FormatLKR(%d) = %q, want %q", in, got, want)
		}
	}
	if got := FormatLKR(math.MinInt64); got != "-Rs 92,233,720,368,547,758.08" {
		t.Errorf("MinInt64 = %q", got)
	}
}
