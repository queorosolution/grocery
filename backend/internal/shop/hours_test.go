package shop

import (
	"testing"
	"time"

	"gp/internal/shared/enum"
)

func TestParseClock(t *testing.T) {
	tests := []struct {
		in      string
		allow24 bool
		want    int
		ok      bool
	}{
		{"00:00", false, 0, true},
		{"08:30", false, 510, true},
		{"23:59", false, 1439, true},
		{"24:00", true, 1440, true},
		{"24:00", false, 0, false}, // a shop cannot open at the end of the day
		{"24:01", true, 0, false},
		{"25:00", true, 0, false},
		{"12:60", false, 0, false},
		{"8:30", false, 0, false},
		{"08:3", false, 0, false},
		{"0830", false, 0, false},
		{"-1:30", false, 0, false},
		{"", false, 0, false},
		{"ab:cd", false, 0, false},
	}
	for _, tt := range tests {
		got, ok := parseClock(tt.in, tt.allow24)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("parseClock(%q, %v) = %d, %v; want %d, %v", tt.in, tt.allow24, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFormatClockRoundTrip(t *testing.T) {
	for _, s := range []string{"00:00", "08:05", "21:00", "23:59", "24:00"} {
		m, ok := parseClock(s, true)
		if !ok || formatClock(m) != s {
			t.Errorf("%q -> %d -> %q", s, m, formatClock(m))
		}
	}
}

// colombo builds an instant from a Colombo wall-clock time (UTC+05:30, no DST).
func colombo(y int, mo time.Month, d, h, mi int) time.Time {
	return time.Date(y, mo, d, h, mi, 0, 0, time.FixedZone("Colombo", 5*3600+1800)).UTC()
}

func TestOpenAt(t *testing.T) {
	// 2026-03-02 is a Monday, 2026-03-08 a Sunday.
	hours := weekHours{
		1: {opens: 8 * 60, closes: 21 * 60},    // Monday 08:00 to 21:00
		2: {opens: 0, closes: minutesPerDay},   // Tuesday all day (to midnight)
		7: {opens: 9 * 60, closes: 13*60 + 30}, // Sunday 09:00 to 13:30
	}
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{"before opening", colombo(2026, 3, 2, 7, 59), false},
		{"opening minute is open", colombo(2026, 3, 2, 8, 0), true},
		{"midday", colombo(2026, 3, 2, 12, 30), true},
		{"last open minute", colombo(2026, 3, 2, 20, 59), true},
		{"closing minute is closed", colombo(2026, 3, 2, 21, 0), false},
		{"closed day (Wednesday)", colombo(2026, 3, 4, 12, 0), false},
		{"midnight to midnight: 00:00", colombo(2026, 3, 3, 0, 0), true},
		{"midnight to midnight: 23:59", colombo(2026, 3, 3, 23, 59), true},
		{"Wednesday 00:00 is closed again", colombo(2026, 3, 4, 0, 0), false},
		{"Sunday uses ISO day 7", colombo(2026, 3, 8, 10, 0), true},
		{"Sunday after closing", colombo(2026, 3, 8, 13, 30), false},
		// 18:30 UTC on Sunday is already Monday 00:00 in Colombo: Monday is closed until 08:00.
		{"UTC Sunday evening is Colombo Monday", time.Date(2026, 3, 8, 18, 30, 0, 0, time.UTC), false},
		// 02:30 UTC Monday is 08:00 Colombo Monday: open.
		{"UTC early Monday is Colombo 08:00", time.Date(2026, 3, 2, 2, 30, 0, 0, time.UTC), true},
		// 18:29 UTC Monday is 23:59 Colombo Monday: closed. 18:30 UTC is Tuesday 00:00: open all day.
		{"UTC Monday 18:29 is Colombo 23:59", time.Date(2026, 3, 2, 18, 29, 0, 0, time.UTC), false},
		{"UTC Monday 18:30 is Colombo Tuesday 00:00", time.Date(2026, 3, 2, 18, 30, 0, 0, time.UTC), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hours.openAt(tt.at); got != tt.want {
				t.Fatalf("openAt(%s) = %v, want %v", tt.at.Format(time.RFC3339), got, tt.want)
			}
		})
	}
}

func TestNoHoursMeansClosed(t *testing.T) {
	if (weekHours{}).openAt(time.Now()) {
		t.Fatal("a shop with no hours must be closed")
	}
}

func readySnapshot() snapshot {
	return snapshot{
		Status: enum.ShopStatusActive, Accepting: true, DeliveryType: enum.DeliveryTypeSlot,
		Hours: weekHours{1: {opens: 8 * 60, closes: 21 * 60}}, PaymentUsable: true, HasFutureSlot: true,
	}
}

func TestSetupReadiness(t *testing.T) {
	tests := []struct {
		name   string
		change func(*snapshot)
		want   []Missing
	}{
		{"all set", func(*snapshot) {}, []Missing{}},
		{"draft shop", func(s *snapshot) { s.Status = enum.ShopStatusDraft }, []Missing{MissingStatus}},
		{"suspended shop", func(s *snapshot) { s.Status = enum.ShopStatusSuspended }, []Missing{MissingStatus}},
		{"no hours", func(s *snapshot) { s.Hours = weekHours{} }, []Missing{MissingHours}},
		{"no payment", func(s *snapshot) { s.PaymentUsable = false }, []Missing{MissingPayment}},
		{"no slot", func(s *snapshot) { s.HasFutureSlot = false }, []Missing{MissingSlot}},
		{"courier needs no slot", func(s *snapshot) { s.HasFutureSlot = false; s.DeliveryType = enum.DeliveryTypeCourier }, []Missing{}},
		{"everything missing", func(s *snapshot) { *s = snapshot{DeliveryType: enum.DeliveryTypeSlot} },
			[]Missing{MissingStatus, MissingHours, MissingPayment, MissingSlot}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := readySnapshot()
			tt.change(&s)
			got := s.setup()
			if got.Ready != (len(tt.want) == 0) || len(got.Missing) != len(tt.want) {
				t.Fatalf("setup() = %+v, want missing %v", got, tt.want)
			}
			for i := range tt.want {
				if got.Missing[i] != tt.want[i] {
					t.Fatalf("missing = %v, want %v", got.Missing, tt.want)
				}
			}
		})
	}
}

func TestCanTakeOrders(t *testing.T) {
	monday := colombo(2026, 3, 2, 12, 0)
	tests := []struct {
		name   string
		change func(*snapshot)
		at     time.Time
		want   Verdict
	}{
		{"open and ready", func(*snapshot) {}, monday, CanOrder},
		{"switched off", func(s *snapshot) { s.Accepting = false }, monday, Closed},
		{"outside hours", func(*snapshot) {}, colombo(2026, 3, 2, 22, 0), Closed},
		{"closed day", func(*snapshot) {}, colombo(2026, 3, 4, 12, 0), Closed},
		{"exactly at closing", func(*snapshot) {}, colombo(2026, 3, 2, 21, 0), Closed},
		{"draft", func(s *snapshot) { s.Status = enum.ShopStatusDraft }, monday, NotReady},
		{"no payment beats closed", func(s *snapshot) { s.PaymentUsable = false; s.Accepting = false }, monday, NotReady},
		{"slots ran out", func(s *snapshot) { s.HasFutureSlot = false }, monday, NotReady},
		{"courier without slots", func(s *snapshot) { s.HasFutureSlot = false; s.DeliveryType = enum.DeliveryTypeCourier }, monday, CanOrder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := readySnapshot()
			tt.change(&s)
			if got := s.canTakeOrders(tt.at); got != tt.want {
				t.Fatalf("canTakeOrders = %q, want %q", got, tt.want)
			}
		})
	}
}
