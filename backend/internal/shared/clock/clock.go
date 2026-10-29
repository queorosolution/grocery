// Package clock is the only place the code reads the time, so tests can fix it.
package clock

import (
	"sync/atomic"
	"time"
)

// Sri Lanka is UTC+05:30 all year (no daylight saving), so a fixed zone avoids
// shipping tzdata in the Lambda binary.
var colombo = time.FixedZone("Asia/Colombo", 5*3600+30*60)

var now atomic.Pointer[func() time.Time]

// Now returns the current time in UTC.
func Now() time.Time {
	if f := now.Load(); f != nil {
		return (*f)().UTC()
	}
	return time.Now().UTC()
}

// Colombo converts t to shop-local time. Opening hours and slots use it.
func Colombo(t time.Time) time.Time { return t.In(colombo) }

// StartOfDay is midnight, Colombo time, of the day t falls in. "Today" for daily
// caps and reports means this day, not the UTC one.
func StartOfDay(t time.Time) time.Time {
	c := Colombo(t)
	return time.Date(c.Year(), c.Month(), c.Day(), 0, 0, 0, 0, colombo)
}

// Freeze makes Now return t until the returned function is called. For tests.
func Freeze(t time.Time) (restore func()) {
	f := func() time.Time { return t }
	prev := now.Swap(&f)
	return func() { now.Store(prev) }
}

// ParseDay reads a YYYY-MM-DD date as midnight, Colombo time. Reports and
// dashboards take their ranges this way.
func ParseDay(s string) (time.Time, error) {
	return time.ParseInLocation(time.DateOnly, s, colombo)
}
