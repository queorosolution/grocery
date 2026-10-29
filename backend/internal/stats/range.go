package stats

import (
	"time"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
)

// maxRangeDays bounds one dashboard; a year of rows is still a few hundred per shop.
const maxRangeDays = 366

// Range is a span of Colombo days, both included. From and To are midnights.
type Range struct{ From, To time.Time }

// parseRange reads the query. Without dates it is the last 7 days, today included.
func parseRange(fromText, toText string) (Range, error) {
	to := clock.StartOfDay(clock.Now())
	var err error
	if toText != "" {
		if to, err = clock.ParseDay(toText); err != nil {
			return Range{}, apperr.Validation(map[string]string{"to": "invalid"})
		}
	}
	from := to.AddDate(0, 0, -6)
	if fromText != "" {
		if from, err = clock.ParseDay(fromText); err != nil {
			return Range{}, apperr.Validation(map[string]string{"from": "invalid"})
		}
	}
	switch {
	case from.After(to):
		return Range{}, apperr.Validation(map[string]string{"from": "after_to"})
	case to.Sub(from) >= maxRangeDays*24*time.Hour:
		return Range{}, apperr.Validation(map[string]string{"from": "range"})
	}
	return Range{From: from, To: to}, nil
}

// End is the first instant after the last day, for a half-open time filter.
func (r Range) End() time.Time { return r.To.AddDate(0, 0, 1) }

func (r Range) fromText() string { return r.From.Format(time.DateOnly) }
func (r Range) toText() string   { return r.To.Format(time.DateOnly) }
