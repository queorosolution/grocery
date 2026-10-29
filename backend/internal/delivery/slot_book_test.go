package delivery_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/delivery"
	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/test/apptest"
)

func (f fixture) book(shopID, slotID uuid.UUID, now time.Time) error {
	return f.a.Env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		return delivery.Book(ctx, tx, shopID, slotID, now)
	})
}

func (f fixture) release(slotID uuid.UUID) error {
	return f.a.Env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		return delivery.Release(ctx, tx, slotID)
	})
}

func (f fixture) booked(slotID uuid.UUID) int {
	return f.a.Count(`SELECT booked FROM slots WHERE id = $1`, slotID)
}

// TestSlot_Race is BIZ-style: many buyers want the last places at the same
// moment. The atomic UPDATE must let exactly `capacity` of them in.
func TestSlot_Race(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	const capacity, buyers = 5, 40
	slot := f.createSlot(shopID, 26*time.Hour, capacity)

	var wg sync.WaitGroup
	results := make(chan error, buyers)
	start := make(chan struct{})
	for i := 0; i < buyers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- f.book(shopID, slot.ID, time.Now())
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	won, full := 0, 0
	for err := range results {
		switch {
		case err == nil:
			won++
		case apperr.Is(err, apperr.SlotFull):
			full++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if won != capacity || full != buyers-capacity {
		t.Errorf("won %d, full %d; want %d and %d", won, full, capacity, buyers-capacity)
	}
	if got := f.booked(slot.ID); got != capacity {
		t.Errorf("booked = %d, want %d", got, capacity)
	}
}

func TestBook_Refusals(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	other := f.activeShop("other-shop")
	slot := f.createSlot(shopID, 26*time.Hour, 1)

	if err := f.book(other, slot.ID, time.Now()); !apperr.Is(err, apperr.NotFound) {
		t.Errorf("another shop's slot: %v, want NOT_FOUND", err)
	}
	if err := f.book(shopID, uuid.New(), time.Now()); !apperr.Is(err, apperr.NotFound) {
		t.Errorf("unknown slot: %v, want NOT_FOUND", err)
	}
	if err := f.book(shopID, slot.ID, slot.CutoffAt); !apperr.Is(err, apperr.SlotClosed) {
		t.Errorf("at the cutoff: %v, want SLOT_CLOSED", err)
	}
	f.OK(http.MethodPatch, "/admin/slots/"+slot.ID.String(), `{"status":"CLOSED"}`)
	if err := f.book(shopID, slot.ID, time.Now()); !apperr.Is(err, apperr.SlotClosed) {
		t.Errorf("switched off: %v, want SLOT_CLOSED", err)
	}
	f.OK(http.MethodPatch, "/admin/slots/"+slot.ID.String(), `{"status":"OPEN"}`)
	if err := f.book(shopID, slot.ID, time.Now()); err != nil {
		t.Fatalf("last place: %v", err)
	}
	if err := f.book(shopID, slot.ID, time.Now()); !apperr.Is(err, apperr.SlotFull) {
		t.Errorf("full: %v, want SLOT_FULL", err)
	}
	if got := f.booked(slot.ID); got != 1 {
		t.Errorf("booked = %d after refusals, want 1", got)
	}
}

func TestRelease(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	slot := f.createSlot(shopID, 26*time.Hour, 2)
	if err := f.book(shopID, slot.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.release(slot.ID); err != nil || f.booked(slot.ID) != 0 {
		t.Fatalf("release: %v, booked %d", err, f.booked(slot.ID))
	}
	// A second release is a bug in the caller: it must fail loudly, not hide as 0.
	err := f.release(slot.ID)
	if err == nil || apperr.From(err).Code != apperr.Internal {
		t.Errorf("double release: %v, want an internal error", err)
	}
	// Release works after the cutoff or after the slot is closed: cancelling must always free the place.
	f.a.Exec(`UPDATE slots SET booked = 1, status = 'CLOSED', cutoff_at = starts_at - interval '2 days'`)
	if err := f.release(slot.ID); err != nil || f.booked(slot.ID) != 0 {
		t.Errorf("release of a closed slot: %v", err)
	}
}

// A release inside a transaction that rolls back must not change the count.
func TestBookRollsBackWithItsTransaction(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	slot := f.createSlot(shopID, 26*time.Hour, 3)
	boom := errors.New("later step failed")
	err := f.a.Env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
		if err := delivery.Book(ctx, tx, shopID, slot.ID, time.Now()); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) || f.booked(slot.ID) != 0 {
		t.Fatalf("err %v, booked %d: a failed order must not keep its place", err, f.booked(slot.ID))
	}
}

func TestPublicSlots(t *testing.T) {
	f := newFixture(t)
	shopID := f.activeShop("krishna-super")
	draft := f.a.SeedShop("not-yet") // DRAFT shops have no public slots
	open := f.createSlot(shopID, 26*time.Hour, 3)
	full := f.createSlot(shopID, 28*time.Hour, 1)
	f.a.Exec(`UPDATE slots SET booked = 5 WHERE id = $1`, full.ID) // over capacity after a capacity cut: remaining stays 0
	closed := f.createSlot(shopID, 30*time.Hour, 3)
	f.OK(http.MethodPatch, "/admin/slots/"+closed.ID.String(), `{"status":"CLOSED"}`)
	f.createSlot(shopID, 32*time.Hour, 3)
	f.a.Exec(`UPDATE slots SET cutoff_at = now() - interval '1 minute'
		WHERE shop_id = $1 AND starts_at > now() + interval '31 hours'`, shopID) // cutoff passed
	f.createSlot(shopID, 24*5*time.Hour, 3) // beyond the 3-day window
	f.createSlot(draft, 26*time.Hour, 3)

	get := func(path string) (int, []delivery.PublicSlot) {
		w := f.a.Call(http.MethodGet, "/v1"+path, "") // no cookies: public
		var body struct{ Data []delivery.PublicSlot }
		if w.Code == http.StatusOK {
			body = apptest.Decode[struct{ Data []delivery.PublicSlot }](t, w)
		}
		return w.Code, body.Data
	}
	code, slots := get("/shops/krishna-super/slots")
	if code != http.StatusOK || len(slots) != 2 {
		t.Fatalf("slots = %d %+v, want the open and the full slot only", code, slots)
	}
	if slots[0].ID != open.ID || slots[0].Remaining != 3 || slots[1].ID != full.ID || slots[1].Remaining != 0 {
		t.Errorf("slots = %+v", slots)
	}
	if _, wide := get("/shops/krishna-super/slots?days=7"); len(wide) != 3 {
		t.Errorf("days=7 gave %d slots, want 3 (the 5-day slot joins)", len(wide))
	}
	if c, _ := get("/shops/not-yet/slots"); c != http.StatusNotFound {
		t.Errorf("draft shop: %d, want 404", c)
	}
	if c, _ := get("/shops/nobody/slots"); c != http.StatusNotFound {
		t.Errorf("unknown shop: %d, want 404", c)
	}
	for _, q := range []string{"days=0", "days=8", "days=x", "days=-1"} {
		if c, _ := get("/shops/krishna-super/slots?" + q); c != http.StatusBadRequest {
			t.Errorf("%s: %d, want 400", q, c)
		}
	}
	w := f.a.Call(http.MethodGet, "/v1/shops/krishna-super/slots", "")
	if w.Header().Get("Cache-Control") == "" || w.Body.String() == "" {
		t.Error("response must carry the default no-store header")
	}
}
