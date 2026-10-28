package delivery_test

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/delivery"
	"gp/test/apptest"
)

func slotsPath(id uuid.UUID, rest string) string {
	return "/admin/shops/" + id.String() + "/slots" + rest
}

func TestSlots_CreateListUpdate(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	slot := f.createSlot(shopID, 26*time.Hour, 5)
	if slot.Capacity != 5 || slot.Booked != 0 || slot.Status != "OPEN" || slot.ShopID != shopID {
		t.Fatalf("created = %+v", slot)
	}

	day := time.Now().Add(26 * time.Hour).In(time.FixedZone("Colombo", 5*3600+1800)).Format(time.DateOnly)
	list := apptest.Decode[struct{ Data []delivery.Slot }](t, f.OK(http.MethodGet, slotsPath(shopID, "?from="+day+"&to="+day), ""))
	if len(list.Data) != 1 || list.Data[0].ID != slot.ID {
		t.Fatalf("list = %+v", list.Data)
	}

	// Lowering capacity below booked is allowed (docs/03 section 10); the slot just shows full.
	f.a.Exec(`UPDATE slots SET booked = 4 WHERE id = $1`, slot.ID)
	patched := apptest.Decode[delivery.Slot](t, f.OK(http.MethodPatch, "/admin/slots/"+slot.ID.String(), `{"capacity":2,"status":"CLOSED"}`))
	if patched.Capacity != 2 || patched.Booked != 4 || patched.Status != "CLOSED" {
		t.Fatalf("patched = %+v", patched)
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE entity = 'slot'`); n != 2 {
		t.Errorf("audit rows = %d, want 2 (create, update)", n)
	}
}

func TestSlots_CreateRefusesBadInput(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	start := time.Now().Add(48 * time.Hour).Truncate(time.Minute)
	one := func(startsAt, endsAt, cutoff time.Time, capacity int) string {
		return `{"slots":[{"startsAt":"` + rfc(startsAt) + `","endsAt":"` + rfc(endsAt) + `","cutoffAt":"` + rfc(cutoff) +
			`","capacity":` + strconv.Itoa(capacity) + `}]}`
	}
	cases := map[string]struct{ body, field string }{
		"in the past":       {one(time.Now().Add(-time.Hour), start, start.Add(-3*time.Hour), 5), "slots.0.startsAt"},
		"too far ahead":     {one(time.Now().AddDate(0, 0, 91), time.Now().AddDate(0, 0, 92), time.Now().AddDate(0, 0, 90), 5), "slots.0.startsAt"},
		"ends before start": {one(start, start.Add(-time.Hour), start.Add(-2*time.Hour), 5), "slots.0.endsAt"},
		"longer than a day": {one(start, start.Add(25*time.Hour), start.Add(-time.Hour), 5), "slots.0.endsAt"},
		"cutoff after":      {one(start, start.Add(time.Hour), start.Add(time.Minute), 5), "slots.0.cutoffAt"},
		"capacity too big":  {one(start, start.Add(time.Hour), start.Add(-time.Hour), 501), "slots.0.capacity"},
		"capacity negative": {one(start, start.Add(time.Hour), start.Add(-time.Hour), -1), "slots.0.capacity"},
		"empty":             {`{"slots":[]}`, "slots"},
		"missing times":     {`{"slots":[{"capacity":5}]}`, "slots.0.startsAt"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := f.Expect(http.MethodPost, slotsPath(shopID, ""), c.body, http.StatusBadRequest, "VALIDATION_ERROR")
			if e.Error.Fields[c.field] == "" {
				t.Errorf("fields = %v, want %q", e.Error.Fields, c.field)
			}
		})
	}
	if n := f.a.Count(`SELECT count(*) FROM slots`); n != 0 {
		t.Errorf("%d slots saved from refused requests", n)
	}
}

func TestSlots_CreateIsAllOrNothing(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	f.createSlot(shopID, 30*time.Hour, 5)
	existing := time.Now().Add(30 * time.Hour).Truncate(time.Minute)
	fresh := existing.Add(3 * time.Hour)
	body := `{"slots":[` + slotJSON(fresh) + `,` + slotJSON(existing) + `]}`
	f.Expect(http.MethodPost, slotsPath(shopID, ""), body, http.StatusConflict, "CONFLICT")
	if n := f.a.Count(`SELECT count(*) FROM slots`); n != 1 {
		t.Errorf("slots = %d, want 1: a clash must undo the whole request", n)
	}
	dup := `{"slots":[` + slotJSON(fresh) + `,` + slotJSON(fresh) + `]}`
	e := f.Expect(http.MethodPost, slotsPath(shopID, ""), dup, http.StatusBadRequest, "VALIDATION_ERROR")
	if e.Error.Fields["slots.1.startsAt"] != "duplicate" {
		t.Errorf("fields = %v", e.Error.Fields)
	}
	many := `{"slots":[` + slotJSON(fresh)
	for i := 0; i < 50; i++ {
		many += `,` + slotJSON(fresh.Add(time.Duration(i+1)*time.Hour))
	}
	f.Expect(http.MethodPost, slotsPath(shopID, ""), many+`]}`, http.StatusBadRequest, "VALIDATION_ERROR")
}

func slotJSON(start time.Time) string {
	return `{"startsAt":"` + rfc(start) + `","endsAt":"` + rfc(start.Add(time.Hour)) + `","cutoffAt":"` + rfc(start.Add(-time.Hour)) + `","capacity":3}`
}

func TestSlots_UpdateRefusesBadInput(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	slot := f.createSlot(shopID, 26*time.Hour, 5)
	path := "/admin/slots/" + slot.ID.String()
	f.Expect(http.MethodPatch, path, `{}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.Expect(http.MethodPatch, path, `{"capacity":501}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.Expect(http.MethodPatch, path, `{"status":"PAUSED"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.Expect(http.MethodPatch, path, `{"startsAt":"2030-01-01T00:00:00Z"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	late := rfc(slot.StartsAt.Add(time.Minute))
	e := f.Expect(http.MethodPatch, path, `{"cutoffAt":"`+late+`"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	if e.Error.Fields["cutoffAt"] != "after_start" {
		t.Errorf("fields = %v", e.Error.Fields)
	}
	f.Expect(http.MethodPatch, "/admin/slots/"+uuid.NewString(), `{"capacity":1}`, http.StatusNotFound, "NOT_FOUND")
	f.Expect(http.MethodPatch, "/admin/slots/nope", `{"capacity":1}`, http.StatusNotFound, "NOT_FOUND")
}

func TestSlots_AdminListRange(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	f.Expect(http.MethodGet, slotsPath(uuid.New(), ""), "", http.StatusNotFound, "NOT_FOUND")
	f.Expect(http.MethodGet, slotsPath(shopID, "?from=yesterday"), "", http.StatusBadRequest, "VALIDATION_ERROR")
	f.Expect(http.MethodGet, slotsPath(shopID, "?from=2026-11-10&to=2026-11-05"), "", http.StatusBadRequest, "VALIDATION_ERROR")
	f.Expect(http.MethodGet, slotsPath(shopID, "?from=2026-11-01&to=2026-12-15"), "", http.StatusBadRequest, "VALIDATION_ERROR")
	f.OK(http.MethodGet, slotsPath(shopID, ""), "")
	f.OK(http.MethodGet, slotsPath(shopID, "?from=2026-11-01&to=2026-12-01"), "") // 31 days: the longest allowed
}

func TestSlots_CopyWeek(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	// A Monday-to-Sunday week in Colombo; the slots sit at 10:00 on day 1 and 3.
	loc := time.FixedZone("Colombo", 5*3600+1800)
	week := time.Now().In(loc).AddDate(0, 0, 14)
	week = time.Date(week.Year(), week.Month(), week.Day(), 0, 0, 0, 0, loc)
	for _, off := range []int{1, 3} {
		start := week.AddDate(0, 0, off).Add(10 * time.Hour)
		f.OK(http.MethodPost, slotsPath(shopID, ""), `{"slots":[`+slotJSON(start)+`]}`)
	}
	f.a.Exec(`UPDATE slots SET booked = 2, status = 'CLOSED'`) // a copy starts empty and open

	copied := apptest.Decode[delivery.CopyWeekResult](t, f.OK(http.MethodPost, slotsPath(shopID, "/copy-week"), `{"weekStart":"`+week.Format(time.DateOnly)+`"}`))
	if copied.Created != 2 || len(copied.Slots) != 2 {
		t.Fatalf("copied = %+v", copied)
	}
	for _, s := range copied.Slots {
		if s.Booked != 0 || s.Status != "OPEN" || s.Capacity != 3 {
			t.Errorf("copy = %+v, want empty, open, same capacity", s)
		}
	}
	if got := copied.Slots[0].StartsAt.Sub(week.AddDate(0, 0, 1).Add(10 * time.Hour)); got != 7*24*time.Hour {
		t.Errorf("first copy starts %v after the source, want 7 days", got)
	}
	// Running it again adds nothing.
	again := apptest.Decode[delivery.CopyWeekResult](t, f.OK(http.MethodPost, slotsPath(shopID, "/copy-week"), `{"weekStart":"`+week.Format(time.DateOnly)+`"}`))
	if again.Created != 0 {
		t.Errorf("second copy created %d", again.Created)
	}
	f.Expect(http.MethodPost, slotsPath(shopID, "/copy-week"), `{"weekStart":"soon"}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.Expect(http.MethodPost, slotsPath(uuid.New(), "/copy-week"), `{"weekStart":"2026-11-02"}`, http.StatusNotFound, "NOT_FOUND")
}
