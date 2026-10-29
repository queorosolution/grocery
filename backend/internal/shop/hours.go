package shop

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gp/internal/shared/clock"
)

// minutesPerDay is also the value of "24:00", the only way to say "until midnight".
const minutesPerDay = 24 * 60

// span is one open day in minutes since local midnight, opens < closes.
type span struct{ opens, closes int }

// weekHours maps ISO weekday (1 to 7) to its span. A missing day is closed.
type weekHours map[int]span

// parseClock reads "HH:MM". "24:00" is accepted only when allow24 is set (for
// closing times), because a shop cannot open at the end of the day.
func parseClock(s string, allow24 bool) (int, bool) {
	h, m, ok := strings.Cut(s, ":")
	if !ok || len(h) != 2 || len(m) != 2 {
		return 0, false
	}
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if err1 != nil || err2 != nil || hh < 0 || mm < 0 || mm > 59 {
		return 0, false
	}
	total := hh*60 + mm
	if total > minutesPerDay || (total == minutesPerDay && !allow24) || (hh == 24 && mm != 0) {
		return 0, false
	}
	return total, true
}

func formatClock(minutes int) string { return fmt.Sprintf("%02d:%02d", minutes/60, minutes%60) }

// openAt reports whether the shop is open at t, in Colombo time. The closing
// minute is not open: a shop that closes at 21:00 is closed at 21:00:00.
func (w weekHours) openAt(t time.Time) bool {
	local := clock.Colombo(t)
	day, ok := w[isoWeekday(local.Weekday())]
	if !ok {
		return false
	}
	now := local.Hour()*60 + local.Minute()
	return now >= day.opens && now < day.closes
}

func isoWeekday(d time.Weekday) int {
	if d == time.Sunday {
		return 7
	}
	return int(d)
}
