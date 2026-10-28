package user_test

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"gp/internal/user"
	"gp/test/apptest"
)

func (f fixture) list(c apptest.Client) []user.Address {
	f.a.T.Helper()
	return apptest.Decode[struct{ Data []user.Address }](f.a.T, c.OK(http.MethodGet, "/me/addresses", "")).Data
}

func TestAddress_FirstIsDefaultAndDefaultMoves(t *testing.T) {
	f := newFixture(t)
	home := f.addAddress(f.me, "Home", "")
	if !home.Address.IsDefault || home.Address.Landmark != nil {
		t.Fatalf("first address = %+v, want default", home.Address)
	}
	work := f.addAddress(f.me, "Work", `"landmark":"  Next to the bank  "`)
	if work.Address.IsDefault || work.Address.Landmark == nil || *work.Address.Landmark != "Next to the bank" {
		t.Fatalf("second address = %+v", work.Address)
	}
	third := f.addAddress(f.me, "Mum", `"isDefault":true`)
	if !third.Address.IsDefault {
		t.Fatal("isDefault must make the new address the default")
	}
	list := f.list(f.me)
	if len(list) != 3 || list[0].ID != third.Address.ID {
		t.Fatalf("list = %+v, want the default first", list)
	}
	defaults := 0
	for _, a := range list {
		if a.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Errorf("%d defaults, want exactly 1", defaults)
	}

	// Moving the default with PATCH.
	updated := apptest.Decode[user.AddressResult](t, f.me.OK(http.MethodPatch, "/me/addresses/"+home.Address.ID.String(), `{"isDefault":true}`))
	if !updated.Address.IsDefault || f.list(f.me)[0].ID != home.Address.ID {
		t.Error("PATCH isDefault must move the default")
	}
	// The default cannot be switched off; move it instead.
	f.me.Expect(http.MethodPatch, "/me/addresses/"+home.Address.ID.String(), `{"isDefault":false}`, http.StatusBadRequest, "VALIDATION_ERROR")
}

func TestAddress_DeleteDefaultPromotesNewest(t *testing.T) {
	f := newFixture(t)
	home := f.addAddress(f.me, "Home", "").Address
	f.addAddress(f.me, "Work", "")
	newest := f.addAddress(f.me, "Mum", "").Address
	f.me.OK(http.MethodDelete, "/me/addresses/"+home.ID.String(), "")
	list := f.list(f.me)
	if len(list) != 2 || list[0].ID != newest.ID || !list[0].IsDefault {
		t.Fatalf("list = %+v, want the newest address as default", list)
	}
	f.me.OK(http.MethodDelete, "/me/addresses/"+newest.ID.String(), "")
	f.me.OK(http.MethodDelete, "/me/addresses/"+f.list(f.me)[0].ID.String(), "")
	if len(f.list(f.me)) != 0 {
		t.Error("all addresses should be gone")
	}
	f.me.Expect(http.MethodDelete, "/me/addresses/"+home.ID.String(), "", http.StatusNotFound, "NOT_FOUND")
}

// FR-ADDR-03: at most 5, even when requests arrive together.
func TestAddress_LimitHoldsUnderConcurrency(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	codes := make(chan int, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- f.me.Call(http.MethodPost, "/me/addresses", addressBody("Spot", `"isDefault":true`)).Code
		}()
	}
	wg.Wait()
	close(codes)
	created := 0
	for c := range codes {
		if c == http.StatusCreated {
			created++
		} else if c != http.StatusBadRequest {
			t.Errorf("unexpected status %d", c)
		}
	}
	if created != 5 {
		t.Errorf("%d created, want 5", created)
	}
	if n := f.a.Count(`SELECT count(*) FROM addresses WHERE is_default`); n != 1 {
		t.Errorf("%d defaults after the race, want 1", n)
	}
	e := f.me.Expect(http.MethodPost, "/me/addresses", addressBody("Sixth", ""), http.StatusBadRequest, "VALIDATION_ERROR")
	if e.Error.Fields["addresses"] != "max_5" {
		t.Errorf("fields = %v", e.Error.Fields)
	}
}

// FR-ADDR-04 and the field limits.
func TestAddress_Validation(t *testing.T) {
	f := newFixture(t)
	body := func(label, line1, lat, lng string) string {
		return `{"label":"` + label + `","line1":"` + line1 + `","lat":` + lat + `,"lng":` + lng + `}`
	}
	cases := map[string]struct{ body, field string }{
		"no label":      {body("", "12 Galle Road", "6.9", "79.8"), "label"},
		"long label":    {body(strings.Repeat("a", 31), "12 Galle Road", "6.9", "79.8"), "label"},
		"short line":    {body("Home", "ab", "6.9", "79.8"), "line1"},
		"long line":     {body("Home", strings.Repeat("a", 161), "6.9", "79.8"), "line1"},
		"chennai":       {body("Home", "12 Galle Road", "13.08", "80.27"), "lat"},
		"too far east":  {body("Home", "12 Galle Road", "7.0", "82.5"), "lng"},
		"null island":   {body("Home", "12 Galle Road", "0", "0"), "lat"},
		"long landmark": {`{"label":"Home","line1":"12 Galle Road","lat":6.9,"lng":79.8,"landmark":"` + strings.Repeat("a", 121) + `"}`, "landmark"},
		"control chars": {body("Ho\\u0000me", "12 Galle Road", "6.9", "79.8"), "label"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			e := f.me.Expect(http.MethodPost, "/me/addresses", c.body, http.StatusBadRequest, "VALIDATION_ERROR")
			if e.Error.Fields[c.field] == "" {
				t.Errorf("fields = %v, want %q", e.Error.Fields, c.field)
			}
		})
	}
	f.me.Expect(http.MethodPost, "/me/addresses", `{"label":"Home","line1":"12 Galle Road","lat":6.9,"lng":79.8,"userId":"`+uuid.NewString()+`"}`,
		http.StatusBadRequest, "VALIDATION_ERROR") // the owner never comes from the body
	if n := f.a.Count(`SELECT count(*) FROM addresses`); n != 0 {
		t.Errorf("%d addresses saved from refused requests", n)
	}

	home := f.addAddress(f.me, "Home", "").Address
	path := "/me/addresses/" + home.ID.String()
	f.me.Expect(http.MethodPatch, path, `{}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPatch, path, `{"lat":13.0}`, http.StatusBadRequest, "VALIDATION_ERROR")
	f.me.Expect(http.MethodPatch, path, `{"label":""}`, http.StatusBadRequest, "VALIDATION_ERROR")
	patched := apptest.Decode[user.AddressResult](t, f.me.OK(http.MethodPatch, path, `{"landmark":"Blue gate","label":" Flat "}`))
	if patched.Address.Label != "Flat" || *patched.Address.Landmark != "Blue gate" || patched.Address.Line1 != home.Line1 {
		t.Errorf("patched = %+v", patched.Address)
	}
	cleared := apptest.Decode[user.AddressResult](t, f.me.OK(http.MethodPatch, path, `{"landmark":""}`))
	if cleared.Address.Landmark != nil {
		t.Errorf("an empty landmark must clear it: %+v", cleared.Address)
	}
}

// Another customer's address is a 404, never a 403, and is never changed (BOLA).
func TestAddress_OtherUsersAreInvisible(t *testing.T) {
	f := newFixture(t)
	other := f.another("kamal")
	theirs := f.addAddress(other, "Theirs", "").Address
	path := "/me/addresses/" + theirs.ID.String()

	f.me.Expect(http.MethodPatch, path, `{"label":"Mine now"}`, http.StatusNotFound, "NOT_FOUND")
	f.me.Expect(http.MethodDelete, path, "", http.StatusNotFound, "NOT_FOUND")
	f.me.Expect(http.MethodPatch, "/me/addresses/not-a-uuid", `{"label":"x"}`, http.StatusNotFound, "NOT_FOUND")
	if got := f.list(f.me); len(got) != 0 {
		t.Errorf("list leaks %+v", got)
	}
	if got := f.list(other); len(got) != 1 || got[0].Label != "Theirs" {
		t.Errorf("their address changed: %+v", got)
	}
}

// FR-ADDR-02: which active shops deliver to the pin.
func TestAddress_DeliveringShops(t *testing.T) {
	f := newFixture(t)
	f.shopWithRule("near-shop") // seeded at 6.9271, 79.8612
	far := f.a.SeedShop("far-shop")
	f.a.Exec(`UPDATE shops SET status = 'ACTIVE', lat = 7.2906, lng = 80.6337 WHERE id = $1`, far) // Kandy
	f.a.SeedShop("draft-shop")                                                                     // DRAFT: never listed

	res := f.addAddress(f.me, "Home", "")
	if len(res.DeliveringShops) != 1 || res.DeliveringShops[0].Slug != "near-shop" {
		t.Fatalf("shops = %+v, want only near-shop", res.DeliveringShops)
	}
	if d := res.DeliveringShops[0].DistanceKm; d < 100 || d > 200 {
		t.Errorf("distance = %d hundredths of a km, want about 1.2 km", d)
	}

	// Move the pin to Kandy: the other shop now delivers, the first no longer does.
	moved := apptest.Decode[user.AddressResult](t, f.me.OK(http.MethodPatch, "/me/addresses/"+res.Address.ID.String(), `{"lat":7.2906,"lng":80.6337}`))
	if len(moved.DeliveringShops) != 1 || moved.DeliveringShops[0].Slug != "far-shop" {
		t.Errorf("shops after the move = %+v", moved.DeliveringShops)
	}

	// A shop's own, smaller range wins over the platform default.
	f.a.Exec(`INSERT INTO delivery_fee_rules (shop_id, max_km) VALUES ($1, 0.50)`, far)
	tight := apptest.Decode[user.AddressResult](t, f.me.OK(http.MethodPatch, "/me/addresses/"+res.Address.ID.String(), `{"lat":7.2906,"lng":80.64}`))
	if len(tight.DeliveringShops) != 0 {
		t.Errorf("0.5 km rule should not reach 0.6 km away road distance: %+v", tight.DeliveringShops)
	}
}

func TestAddress_NoRuleMeansNoDelivery(t *testing.T) {
	f := newFixture(t)
	id := f.a.SeedShop("no-rule")
	f.a.Exec(`UPDATE shops SET status = 'ACTIVE' WHERE id = $1`, id)
	res := f.addAddress(f.me, "Home", "")
	if res.DeliveringShops == nil || len(res.DeliveringShops) != 0 {
		t.Errorf("shops = %#v, want an empty list (not null)", res.DeliveringShops)
	}
}
