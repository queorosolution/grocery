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

type fixture struct {
	a *apptest.App
	apptest.AdminAPI
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	a := apptest.New(t)
	return fixture{a: a, AdminAPI: a.AsAdmin()}
}

// activeShop seeds an ACTIVE shop; a plain SeedShop is a DRAFT.
func (f fixture) activeShop(slug string) uuid.UUID {
	id := f.a.SeedShop(slug)
	f.a.Exec(`UPDATE shops SET status = 'ACTIVE' WHERE id = $1`, id)
	return id
}

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// slotBody is a create body for one slot starting in `in`, lasting two hours,
// closing orders one hour before the start.
func slotBody(in time.Duration, capacity int) string {
	start := time.Now().Add(in).Truncate(time.Minute)
	return `{"slots":[{"startsAt":"` + rfc(start) + `","endsAt":"` + rfc(start.Add(2*time.Hour)) +
		`","cutoffAt":"` + rfc(start.Add(-time.Hour)) + `","capacity":` + strconv.Itoa(capacity) + `}]}`
}

func (f fixture) createSlot(shopID uuid.UUID, in time.Duration, capacity int) delivery.Slot {
	f.a.T.Helper()
	w := f.OK(http.MethodPost, "/admin/shops/"+shopID.String()+"/slots", slotBody(in, capacity))
	return apptest.Decode[struct{ Data []delivery.Slot }](f.a.T, w).Data[0]
}
