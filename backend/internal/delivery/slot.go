package delivery

import (
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/enum"
	"gp/internal/shared/validate"
)

// Slot is a delivery window as the console sees it.
type Slot struct {
	ID       uuid.UUID       `json:"id"`
	ShopID   uuid.UUID       `json:"shopId"`
	StartsAt time.Time       `json:"startsAt"`
	EndsAt   time.Time       `json:"endsAt"`
	CutoffAt time.Time       `json:"cutoffAt"`
	Capacity int             `json:"capacity"`
	Booked   int             `json:"booked"`
	Status   enum.SlotStatus `json:"status"`
}

// PublicSlot is what customers see: remaining places, never the booked count.
type PublicSlot struct {
	ID        uuid.UUID `json:"id"`
	StartsAt  time.Time `json:"startsAt"`
	EndsAt    time.Time `json:"endsAt"`
	CutoffAt  time.Time `json:"cutoffAt"`
	Remaining int       `json:"remaining"`
}

// SlotInput is one slot in a create request.
type SlotInput struct {
	StartsAt time.Time `json:"startsAt"`
	EndsAt   time.Time `json:"endsAt"`
	CutoffAt time.Time `json:"cutoffAt"`
	Capacity int       `json:"capacity"`
}

// SlotPatch changes a slot. Times other than the cutoff are fixed: orders
// already point at the window. Lowering capacity below booked is allowed; the
// slot then shows as full and existing orders stay (docs/03 section 10).
type SlotPatch struct {
	Capacity *int             `json:"capacity"`
	Status   *enum.SlotStatus `json:"status"`
	CutoffAt *time.Time       `json:"cutoffAt"`
}

const (
	maxSlotsPerCreate = 50
	maxCapacity       = 500
	maxSlotLength     = 24 * time.Hour
	maxDaysAhead      = 90
	maxPublicDays     = 7
	maxAdminRangeDays = 31
	maxCopySlots      = 350 // a full week of one slot every half hour, 7 x 48 = 336
)

func (in SlotInput) validate(prefix string, now time.Time) map[string]string {
	f := map[string]string{}
	bad := func(field, reason string) { f[prefix+field] = reason }
	switch {
	case in.StartsAt.IsZero():
		bad("startsAt", "required")
	case !in.StartsAt.After(now):
		bad("startsAt", "in_past")
	case in.StartsAt.After(now.AddDate(0, 0, maxDaysAhead)):
		bad("startsAt", "too_far")
	}
	switch {
	case in.EndsAt.IsZero():
		bad("endsAt", "required")
	case !in.EndsAt.After(in.StartsAt):
		bad("endsAt", "before_start")
	case in.EndsAt.Sub(in.StartsAt) > maxSlotLength:
		bad("endsAt", "too_long")
	}
	switch {
	case in.CutoffAt.IsZero():
		bad("cutoffAt", "required")
	case in.CutoffAt.After(in.StartsAt):
		bad("cutoffAt", "after_start")
	}
	if in.Capacity < 0 || in.Capacity > maxCapacity {
		bad("capacity", "out_of_range")
	}
	return f
}

func (p SlotPatch) validate() error {
	v := validate.New().Check("body", p.Capacity != nil || p.Status != nil || p.CutoffAt != nil, "empty")
	if p.Capacity != nil {
		v.Int("capacity", int64(*p.Capacity), 0, maxCapacity)
	}
	if p.Status != nil {
		v.Check("status", *p.Status == enum.SlotStatusOpen || *p.Status == enum.SlotStatusClosed, "invalid")
	}
	return v.Err()
}

// colomboDay parses "2026-11-02" as midnight in Colombo.
func colomboDay(field, s string) (time.Time, error) {
	loc := clock.Colombo(clock.Now()).Location()
	t, err := time.ParseInLocation(time.DateOnly, s, loc)
	if err != nil {
		return time.Time{}, apperr.Validation(map[string]string{field: "invalid_date"})
	}
	return t, nil
}

// startOfColomboDay is midnight (Colombo) of the day that contains t.
func startOfColomboDay(t time.Time) time.Time {
	c := clock.Colombo(t)
	return time.Date(c.Year(), c.Month(), c.Day(), 0, 0, 0, 0, c.Location())
}
