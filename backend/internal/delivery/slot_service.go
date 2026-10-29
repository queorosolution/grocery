package delivery

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	"gp/internal/audit"
	"gp/internal/shared/apperr"
	"gp/internal/shared/clock"
	"gp/internal/shared/db"
)

// AdminSlots lists a shop's slots for the Colombo dates from..to (both inclusive).
func (s *Service) AdminSlots(ctx context.Context, scope db.Scope, shopID uuid.UUID, from, to string) ([]Slot, error) {
	start, end, err := adminRange(from, to)
	if err != nil {
		return nil, err
	}
	var out []Slot
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := s.requireShop(ctx, tx, shopID); err != nil {
			return err
		}
		out, err = slotsBetween(ctx, tx, shopID, start, end)
		return err
	})
	return out, err
}

// adminRange turns optional date strings into [start, end). The default is the
// next 7 days; the longest range is 31 days.
func adminRange(from, to string) (time.Time, time.Time, error) {
	start := startOfColomboDay(clock.Now())
	end := start.AddDate(0, 0, 7)
	var err error
	if from != "" {
		if start, err = colomboDay("from", from); err != nil {
			return start, end, err
		}
		end = start.AddDate(0, 0, 7)
	}
	if to != "" {
		var last time.Time
		if last, err = colomboDay("to", to); err != nil {
			return start, end, err
		}
		end = last.AddDate(0, 0, 1)
	}
	if !end.After(start) || end.Sub(start) > maxAdminRangeDays*24*time.Hour {
		return start, end, apperr.Validation(map[string]string{"to": "bad_range"})
	}
	return start, end, nil
}

// CreateSlots adds up to 50 slots in one go; one bad slot refuses them all.
func (s *Service) CreateSlots(ctx context.Context, scope db.Scope, shopID uuid.UUID, in []SlotInput) ([]Slot, error) {
	now := clock.Now()
	fields := map[string]string{}
	if len(in) == 0 || len(in) > maxSlotsPerCreate {
		fields["slots"] = "count_1_to_50"
	}
	seen := map[time.Time]bool{}
	for i, sl := range in {
		for k, v := range sl.validate(prefix(i), now) {
			fields[k] = v
		}
		if seen[sl.StartsAt] {
			fields[prefix(i)+"startsAt"] = "duplicate"
		}
		seen[sl.StartsAt] = true
	}
	if len(fields) > 0 {
		return nil, apperr.Validation(fields)
	}
	out := make([]Slot, 0, len(in))
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		out = out[:0]
		if err := s.requireShop(ctx, tx, shopID); err != nil {
			return err
		}
		for _, sl := range in {
			created, err := insertSlot(ctx, tx, shopID, sl)
			if err != nil {
				return err
			}
			out = append(out, created)
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "slot.create", Entity: "slot", EntityID: shopID.String(),
			ShopID: shopID, After: map[string]int{"count": len(out)}})
	})
	return out, err
}

// UpdateSlot changes capacity, status or cutoff of one slot.
func (s *Service) UpdateSlot(ctx context.Context, scope db.Scope, id uuid.UUID, p SlotPatch) (Slot, error) {
	if err := p.validate(); err != nil {
		return Slot{}, err
	}
	var out Slot
	err := s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		before, found, err := slotByID(ctx, tx, id)
		if err != nil {
			return err
		}
		if !found {
			return apperr.New(apperr.NotFound)
		}
		if out, err = patchSlot(ctx, tx, id, p); err != nil {
			return err
		}
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "slot.update", Entity: "slot", EntityID: id.String(),
			ShopID: before.ShopID, Before: before, After: out})
	})
	return out, err
}

// CopyWeekResult says how many slots were added.
type CopyWeekResult struct {
	Created int    `json:"created"`
	Slots   []Slot `json:"slots"`
}

// CopyWeek copies the seven days starting at weekStart (a Colombo date) into the
// following seven days. Existing slots are left alone, so it can run twice.
func (s *Service) CopyWeek(ctx context.Context, scope db.Scope, shopID uuid.UUID, weekStart string) (CopyWeekResult, error) {
	from, err := colomboDay("weekStart", weekStart)
	if err != nil {
		return CopyWeekResult{}, err
	}
	to := from.AddDate(0, 0, 7)
	var out CopyWeekResult
	err = s.db.Tx(ctx, scope, func(ctx context.Context, tx *db.Tx) error {
		if err := s.requireShop(ctx, tx, shopID); err != nil {
			return err
		}
		n, err := countSlots(ctx, tx, shopID, from, to)
		if err != nil {
			return err
		}
		if n > maxCopySlots {
			return apperr.Validation(map[string]string{"weekStart": "too_many_slots"})
		}
		if out.Slots, err = copyForward(ctx, tx, shopID, from, to, clock.Now()); err != nil {
			return err
		}
		out.Created = len(out.Slots)
		return audit.Write(ctx, tx, scope, audit.Entry{Action: "slot.copy_week", Entity: "slot", EntityID: shopID.String(),
			ShopID: shopID, After: map[string]any{"weekStart": weekStart, "created": out.Created}})
	})
	return out, err
}

// PublicSlots are the windows a customer can pick for the next `days` Colombo
// days (today counts as the first). Only active shops have slots.
func (s *Service) PublicSlots(ctx context.Context, slug string, days int) ([]PublicSlot, error) {
	now := clock.Now()
	end := startOfColomboDay(now).AddDate(0, 0, days)
	var out []PublicSlot
	err := s.db.Tx(ctx, db.Public(), func(ctx context.Context, tx *db.Tx) error {
		id, found, err := s.shops.IDBySlug(ctx, tx, slug)
		if err != nil {
			return err
		}
		if !found {
			return apperr.New(apperr.NotFound)
		}
		out, err = openSlots(ctx, tx, id, now, end)
		return err
	})
	return out, err
}

func prefix(i int) string { return "slots." + strconv.Itoa(i) + "." }
