package catalog_test

import (
	"net/http"
	"testing"
	"time"

	"gp/internal/catalog"
)

const docRule = `{"baseCents":10000,"perKmCents":3000,"freeKm":2,"perKgCents":1000,"freeKg":5,"maxKm":8,"roadFactor":1.3}`

// homeWorld has a near shop, a far shop (Kandy), a draft shop and a default fee rule.
func homeWorld(t *testing.T) (fixture, catalog.Home) {
	t.Helper()
	f := newFixture(t)
	f.admin.OK(http.MethodPut, "/admin/fee-rules/default", docRule)
	near := f.activeShop("near-shop", 6.9271, 79.8612)
	far := f.activeShop("far-shop", 7.2906, 80.6337)
	f.a.SeedShop("draft-shop")
	f.stock(near, "Samba Rice 5kg", 143500, ptr(int64(150000)), true, nil)
	f.stock(near, "Dhal 1kg", 48000, ptr(int64(60000)), true, nil)
	f.stock(near, "Soap", 9000, nil, true, nil)                         // no MRP: no deal
	f.stock(near, "Sold out tea", 10000, ptr(int64(90000)), false, nil) // unavailable: no deal
	f.stock(far, "Far rice", 100000, ptr(int64(130000)), true, nil)
	return f, catalog.Home{}
}

func TestHome_ShopsDealsAndDistance(t *testing.T) {
	f, _ := homeWorld(t)

	all := decode[catalog.Home](f, f.publicGet("/home"))
	if len(all.Shops) != 2 || all.Shops[0].Slug != "far-shop" || all.Shops[1].Slug != "near-shop" || all.Shops[0].DistanceKm != nil {
		t.Fatalf("without a pin: %+v", all.Shops)
	}
	if len(all.Promos) != 0 {
		t.Fatalf("promos = %v", all.Promos)
	}

	w := f.publicGet("/home?lat=6.9271&lng=79.8612")
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=60" {
		t.Errorf("Cache-Control = %q", got)
	}
	near := decode[catalog.Home](f, w)
	if len(near.Shops) != 1 || near.Shops[0].Slug != "near-shop" || near.Shops[0].DistanceKm == nil || *near.Shops[0].DistanceKm > 100 {
		t.Fatalf("with a pin: %+v", near.Shops)
	}
	if len(near.Deals) != 2 || near.Deals[0].Name != "Dhal 1kg" && near.Deals[0].Name != "Samba Rice 5kg" {
		t.Fatalf("deals = %+v", near.Deals)
	}
	if d := near.Deals[0]; d.Name != "Dhal 1kg" || d.SaveCents != 12000 || d.ShopSlug != "near-shop" || d.ShopName != "Shop near-shop" || d.MRPCents != 60000 {
		t.Errorf("best deal = %+v", d)
	}
}

func TestHome_PinIsRoundedToTwoDecimals(t *testing.T) {
	f, _ := homeWorld(t)
	a := f.publicGet("/home?lat=6.9261&lng=79.8612").Body.String()
	b := f.publicGet("/home?lat=6.9299&lng=79.8649").Body.String() // same 6.93 / 79.86
	c := f.publicGet("/home?lat=6.9371&lng=79.8612").Body.String() // 6.94: a different place
	if a != b {
		t.Fatalf("pins in the same 2-decimal cell answer differently:\n%s\n%s", a, b)
	}
	if a == c {
		t.Fatal("a pin 1 km away answers the same: rounding is too coarse")
	}
}

func TestHome_PinRules(t *testing.T) {
	f, _ := homeWorld(t)
	tests := map[string]struct{ query, field, reason string }{
		"only lat":         {"?lat=6.9", "lng", "required"},
		"only lng":         {"?lng=79.8", "lat", "required"},
		"not a number":     {"?lat=abc&lng=79.8", "lat", "invalid"},
		"nan":              {"?lat=NaN&lng=NaN", "lat", "out_of_range"},
		"outside the isle": {"?lat=51.5&lng=-0.12", "lat", "out_of_range"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			w := f.publicGet("/home" + tc.query)
			e := decodeErr(f, w)
			if w.Code != http.StatusBadRequest || e.Error.Fields[tc.field] != tc.reason {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if cc := w.Header().Get("Cache-Control"); cc == "public, max-age=60" {
				t.Fatalf("an error was marked cacheable: %q", cc)
			}
		})
	}
}

func TestHome_OpenNowAndNextSlot(t *testing.T) {
	f, _ := homeWorld(t)
	var near string
	if err := f.a.Env.Owner.QueryRow(f.a.T.Context(), `SELECT id::text FROM shops WHERE slug = 'near-shop'`).Scan(&near); err != nil {
		t.Fatal(err)
	}
	card := func() catalog.ShopCard {
		return decode[catalog.Home](f, f.publicGet("/home?lat=6.9271&lng=79.8612")).Shops[0]
	}
	if c := card(); c.OpenNow || c.NextSlot != nil {
		t.Fatalf("no hours and no slots: %+v", c)
	}

	f.a.Exec(`INSERT INTO shop_hours (shop_id, weekday, opens, closes) SELECT $1, d, '00:00', '24:00' FROM generate_series(1, 7) d`, near)
	later, sooner := time.Now().Add(48*time.Hour).Truncate(time.Second), time.Now().Add(26*time.Hour).Truncate(time.Second)
	for _, at := range []time.Time{later, sooner} {
		f.a.Exec(`INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity) VALUES ($1, $2::timestamptz, $2::timestamptz + interval '2 hours', $2::timestamptz - interval '1 hour', 5)`, near, at)
	}
	// A full slot and a closed slot do not count.
	f.a.Exec(`INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity, booked) VALUES ($1, now() + interval '3 hours', now() + interval '5 hours', now() + interval '1 hour', 2, 2)`, near)
	f.a.Exec(`INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity, status) VALUES ($1, now() + interval '4 hours', now() + interval '6 hours', now() + interval '2 hours', 2, 'CLOSED')`, near)

	c := card()
	if !c.OpenNow || c.NextSlot == nil || !c.NextSlot.Equal(sooner) {
		t.Fatalf("card = %+v, want open with next slot %s", c, sooner)
	}
}
