package clock

import (
	"testing"
	"time"
)

func TestNowIsUTC(t *testing.T) {
	if Now().Location() != time.UTC {
		t.Fatal("Now must be UTC")
	}
}

func TestFreezeAndRestore(t *testing.T) {
	fixed := time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC)
	restore := Freeze(fixed)
	if !Now().Equal(fixed) {
		t.Fatal("Now is not frozen")
	}
	restore()
	if Now().Equal(fixed) {
		t.Fatal("restore did not unfreeze")
	}
}

func TestColomboIsPlus0530(t *testing.T) {
	// 20:00 UTC is 01:30 the next day in Colombo: a midnight edge for opening hours.
	got := Colombo(time.Date(2026, 10, 28, 20, 0, 0, 0, time.UTC))
	if got.Day() != 29 || got.Hour() != 1 || got.Minute() != 30 {
		t.Fatalf("got %s", got)
	}
}

func TestParseDayIsColomboMidnight(t *testing.T) {
	d, err := ParseDay("2026-10-29")
	if err != nil || !d.Equal(time.Date(2026, 10, 28, 18, 30, 0, 0, time.UTC)) {
		t.Fatalf("got %s, %v", d, err)
	}
	for _, bad := range []string{"", "29-10-2026", "2026-13-01", "2026-10-29T00:00:00Z"} {
		if _, err := ParseDay(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
}
