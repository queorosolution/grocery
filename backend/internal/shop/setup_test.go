package shop_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"gp/internal/shared/apperr"
	"gp/internal/shared/db"
	"gp/internal/shared/enum"
	"gp/internal/shop"
	"gp/test/apptest"
)

func TestSettingsRules(t *testing.T) {
	f := newFixture(t)
	id := f.createShop(newShop).Shop.ID
	put := func(mutate func(*shop.Settings)) (int, apptest.APIError) {
		s := validSettings()
		mutate(&s)
		w := f.Call(http.MethodPut, f.shopPath(id, "/settings"), jsonOf(t, s))
		return w.Code, apptest.ErrorOf(f.a.T, w)
	}

	if code, _ := put(func(s *shop.Settings) { s.MarkupBP = 500; s.MinOrderCents = 100000 }); code != http.StatusOK {
		t.Fatalf("valid settings = %d", code)
	}
	d := apptest.Decode[shop.Detail](t, f.OK(http.MethodGet, f.shopPath(id, ""), ""))
	if d.Settings.MarkupBP != 500 || d.Settings.MinOrderCents != 100000 || d.Settings.PaymentProvider != "MANUAL" {
		t.Fatalf("saved = %+v", d.Settings)
	}

	bad := map[string]struct {
		mutate func(*shop.Settings)
		field  string
	}{
		"delivery type":     {func(s *shop.Settings) { s.DeliveryType = "DRONE" }, "deliveryType"},
		"payment provider":  {func(s *shop.Settings) { s.PaymentProvider = "PAYPAL" }, "paymentProvider"},
		"driver source":     {func(s *shop.Settings) { s.DriverSource = "NOBODY" }, "driverSource"},
		"no channels":       {func(s *shop.Settings) { s.NotifyChannels = nil }, "notifyChannels"},
		"unknown channel":   {func(s *shop.Settings) { s.NotifyChannels = []enum.NotifyChannel{"FAX"} }, "notifyChannels"},
		"duplicate channel": {func(s *shop.Settings) { s.NotifyChannels = []enum.NotifyChannel{"SMS", "SMS"} }, "notifyChannels"},
		"markup too high":   {func(s *shop.Settings) { s.MarkupBP = 2001 }, "markupBp"},
		"negative markup":   {func(s *shop.Settings) { s.MarkupBP = -1 }, "markupBp"},
		"negative minimum":  {func(s *shop.Settings) { s.MinOrderCents = -5 }, "minOrderCents"},
		"huge minimum":      {func(s *shop.Settings) { s.MinOrderCents = 1 << 60 }, "minOrderCents"},
		"lines":             {func(s *shop.Settings) { s.MaxLines = 0 }, "maxLines"},
		"review timeout":    {func(s *shop.Settings) { s.ReviewTimeoutMin = 4 }, "reviewTimeoutMin"},
		"payment timeout":   {func(s *shop.Settings) { s.PaymentTimeoutMin = 241 }, "paymentTimeoutMin"},
	}
	for name, c := range bad {
		t.Run(name, func(t *testing.T) {
			code, e := put(c.mutate)
			if code != http.StatusBadRequest || e.Error.Fields[c.field] == "" {
				t.Fatalf("status %d fields %v, want 400 with %q", code, e.Error.Fields, c.field)
			}
		})
	}
	// The cart fields belong to the cart-tenant step, not to this request.
	if w := f.Call(http.MethodPut, f.shopPath(id, "/settings"), `{"cartTenantId":"ten_1"}`); w.Code != http.StatusBadRequest {
		t.Errorf("cartTenantId in settings = %d, want 400", w.Code)
	}
}

func TestSettingsPaymentAccountMustBelong(t *testing.T) {
	f := newFixture(t)
	a := f.createShop(newShop).Shop.ID
	b := f.createShop(`{"slug":"shop-b","name":"Shop B","lat":6.9,"lng":79.9}`).Shop.ID
	mine := apptest.Decode[shop.Account](t, f.OK(http.MethodPost, "/admin/payment-accounts",
		`{"ownerType":"SHOP","shopId":"`+a.String()+`","provider":"MANUAL"}`))

	settings := validSettings()
	settings.PaymentAccountID = &mine.ID
	if w := f.Call(http.MethodPut, f.shopPath(a, "/settings"), jsonOf(t, settings)); w.Code != http.StatusOK {
		t.Fatalf("own account = %d %s", w.Code, w.Body)
	}
	w := f.Call(http.MethodPut, f.shopPath(b, "/settings"), jsonOf(t, settings))
	if w.Code != http.StatusBadRequest || apptest.ErrorOf(f.a.T, w).Error.Fields["paymentAccountId"] != "unknown" {
		t.Fatalf("another shop's account = %d %s", w.Code, w.Body)
	}
	settings.PaymentProvider = "PAYHERE"
	w = f.Call(http.MethodPut, f.shopPath(a, "/settings"), jsonOf(t, settings))
	if w.Code != http.StatusBadRequest || apptest.ErrorOf(f.a.T, w).Error.Fields["paymentProvider"] != "provider_mismatch" {
		t.Fatalf("provider mismatch = %d %s", w.Code, w.Body)
	}
	missing := validSettings()
	id := randomID()
	missing.PaymentAccountID = &id
	if w := f.Call(http.MethodPut, f.shopPath(a, "/settings"), jsonOf(t, missing)); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown account = %d", w.Code)
	}
}

func TestSetHours(t *testing.T) {
	f := newFixture(t)
	id := f.createShop(newShop).Shop.ID
	put := func(body string) *http.Response {
		return f.Call(http.MethodPut, f.shopPath(id, "/hours"), body).Result()
	}

	d := apptest.Decode[shop.Detail](t, f.OK(http.MethodPut, f.shopPath(id, "/hours"),
		`{"days":[{"weekday":1,"opens":"08:00","closes":"21:00"},{"weekday":7,"opens":"09:30","closes":"24:00"}]}`))
	if len(d.Hours) != 2 || d.Hours[0].Weekday != 1 || d.Hours[1].Closes != "24:00" || d.Hours[1].Opens != "09:30" {
		t.Fatalf("hours = %+v", d.Hours)
	}

	bad := map[string]string{
		"closes before opens": `{"days":[{"weekday":1,"opens":"21:00","closes":"08:00"}]}`,
		"closes equals opens": `{"days":[{"weekday":1,"opens":"08:00","closes":"08:00"}]}`,
		"opens at 24:00":      `{"days":[{"weekday":1,"opens":"24:00","closes":"24:00"}]}`,
		"past 24:00":          `{"days":[{"weekday":1,"opens":"08:00","closes":"24:30"}]}`,
		"bad clock":           `{"days":[{"weekday":1,"opens":"8am","closes":"21:00"}]}`,
		"weekday 0":           `{"days":[{"weekday":0,"opens":"08:00","closes":"21:00"}]}`,
		"weekday 8":           `{"days":[{"weekday":8,"opens":"08:00","closes":"21:00"}]}`,
		"duplicate weekday":   `{"days":[{"weekday":1,"opens":"08:00","closes":"12:00"},{"weekday":1,"opens":"13:00","closes":"21:00"}]}`,
		"unknown field":       `{"days":[],"timezone":"UTC"}`,
	}
	for name, body := range bad {
		if got := put(body).StatusCode; got != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, got)
		}
	}
	// A failed request leaves the saved week alone.
	d = apptest.Decode[shop.Detail](t, f.OK(http.MethodGet, f.shopPath(id, ""), ""))
	if len(d.Hours) != 2 {
		t.Fatalf("a refused request changed the hours: %+v", d.Hours)
	}
	// An empty list closes every day.
	d = apptest.Decode[shop.Detail](t, f.OK(http.MethodPut, f.shopPath(id, "/hours"), `{"days":[]}`))
	if len(d.Hours) != 0 {
		t.Fatalf("hours = %+v", d.Hours)
	}
}

func TestReadinessAndAccepting(t *testing.T) {
	f := newFixture(t)
	id := f.createShop(newShop).Shop.ID

	r := apptest.Decode[shop.Readiness](t, f.OK(http.MethodGet, f.shopPath(id, "/readiness"), ""))
	want := []shop.Missing{shop.MissingStatus, shop.MissingHours, shop.MissingPayment, shop.MissingSlot}
	if r.Ready || len(r.Missing) != len(want) {
		t.Fatalf("readiness = %+v, want missing %v", r, want)
	}

	w := f.Call(http.MethodPost, f.shopPath(id, "/accepting"), `{"on":true}`)
	e := apptest.ErrorOf(f.a.T, w)
	if w.Code != http.StatusConflict || e.Error.Code != "SHOP_NOT_READY" || len(e.Error.Fields) != 4 || e.Error.Fields["PAYMENT_ACCOUNT"] != "missing" {
		t.Fatalf("accepting when not ready = %d %s", w.Code, w.Body)
	}
	if n := f.a.Count(`SELECT count(*) FROM shops WHERE id = $1 AND accepting_orders`, id); n != 0 {
		t.Fatal("the switch changed although the shop is not ready")
	}

	ready := f.makeReady("ready-shop")
	d := apptest.Decode[shop.Detail](t, f.OK(http.MethodPost, f.shopPath(ready, "/accepting"), `{"on":true}`))
	if !d.Shop.AcceptingOrders || !d.Readiness.Ready {
		t.Fatalf("detail = %+v", d)
	}
	// Off always works, even when something is missing now.
	f.a.Exec(`DELETE FROM slots WHERE shop_id = $1`, ready)
	d = apptest.Decode[shop.Detail](t, f.OK(http.MethodPost, f.shopPath(ready, "/accepting"), `{"on":false}`))
	if d.Shop.AcceptingOrders || d.Readiness.Ready {
		t.Fatalf("detail after off = %+v", d)
	}
	if n := f.a.Count(`SELECT count(*) FROM audit_log WHERE action = 'shop.accepting' AND shop_id = $1`, ready); n != 2 {
		t.Errorf("accepting audit rows = %d, want 2", n)
	}

	// Slots that are full or past their cutoff do not count; courier shops need none.
	f.a.Exec(`INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity, booked) VALUES ($1, now() + interval '1 day', now() + interval '2 day', now() + interval '20 hours', 5, 5)`, ready)
	f.a.Exec(`INSERT INTO slots (shop_id, starts_at, ends_at, cutoff_at, capacity) VALUES ($1, now() + interval '3 day', now() + interval '4 day', now() - interval '1 hour', 5)`, ready)
	if r := apptest.Decode[shop.Readiness](t, f.OK(http.MethodGet, f.shopPath(ready, "/readiness"), "")); r.Ready {
		t.Fatal("full and closed slots must not make a shop ready")
	}
	s := validSettings()
	s.DeliveryType = "COURIER"
	s.PaymentAccountID = settingsAccount(t, f, ready)
	f.OK(http.MethodPut, f.shopPath(ready, "/settings"), jsonOf(t, s))
	if r := apptest.Decode[shop.Readiness](t, f.OK(http.MethodGet, f.shopPath(ready, "/readiness"), "")); !r.Ready {
		t.Fatalf("a courier shop needs no slot: %+v", r)
	}
}

func settingsAccount(t *testing.T, f fixture, id interface{ String() string }) *uuid.UUID {
	t.Helper()
	var acc uuid.UUID
	if err := f.a.Env.Owner.QueryRow(context.Background(), `SELECT payment_account_id FROM shop_settings WHERE shop_id = $1`, id.String()).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	return &acc
}

// CanTakeOrders is the check the order module runs at submit time (FR-SHOP-04).
func TestCanTakeOrders(t *testing.T) {
	f := newFixture(t)
	id := f.makeReady("krishna-super")
	f.OK(http.MethodPost, f.shopPath(id, "/accepting"), `{"on":true}`)
	svc := shop.NewService(f.a.Env.DB, f.a.Secrets, true)
	check := func(shopID uuid.UUID, at time.Time) error {
		return f.a.Env.DB.Tx(context.Background(), db.System(), func(ctx context.Context, tx *db.Tx) error {
			return svc.CanTakeOrders(ctx, tx, shopID, at)
		})
	}
	now := time.Now()

	if err := check(id, now); err != nil {
		t.Fatalf("open all week and ready: %v", err)
	}
	// Close Wednesdays: an instant on a Wednesday in Colombo is refused with SHOP_CLOSED.
	f.OK(http.MethodPut, f.shopPath(id, "/hours"), `{"days":[{"weekday":1,"opens":"08:00","closes":"21:00"}]}`)
	monday := time.Date(2026, 3, 2, 12, 0, 0, 0, time.FixedZone("Colombo", 5*3600+1800))
	if err := check(id, monday); err != nil {
		t.Errorf("Monday noon: %v", err)
	}
	for name, at := range map[string]time.Time{
		"Monday 21:00 (closing minute)": monday.Add(9 * time.Hour),
		"Tuesday (no hours)":            monday.Add(24 * time.Hour),
	} {
		if err := check(id, at); !apperr.Is(err, apperr.ShopClosed) {
			t.Errorf("%s: %v, want SHOP_CLOSED", name, err)
		}
	}
	// Switched off.
	f.OK(http.MethodPost, f.shopPath(id, "/accepting"), `{"on":false}`)
	if err := check(id, monday); !apperr.Is(err, apperr.ShopClosed) {
		t.Errorf("switched off: %v, want SHOP_CLOSED", err)
	}
	// Setup broken: not ready beats closed.
	f.OK(http.MethodPatch, f.shopPath(id, ""), `{"status":"SUSPENDED"}`)
	if err := check(id, monday); !apperr.Is(err, apperr.ShopNotReady) {
		t.Errorf("suspended: %v, want SHOP_NOT_READY", err)
	}
	if err := check(randomID(), monday); !apperr.Is(err, apperr.NotFound) {
		t.Errorf("unknown shop: %v, want NOT_FOUND", err)
	}
}

// BillFacts is what a revision of an order in flight reads: the rules, with no
// readiness check, so it works for a shop that was suspended since the bill was sent.
func TestBillFacts(t *testing.T) {
	f := newFixture(t)
	id := f.makeReady("krishna-super")
	f.a.Exec(`UPDATE shop_settings SET min_order_cents = 5000, max_lines = 7, min_platform_margin_cents = 300 WHERE shop_id = $1`, id)
	f.a.Exec(`UPDATE shops SET status = 'SUSPENDED' WHERE id = $1`, id)
	svc := shop.NewService(f.a.Env.DB, f.a.Secrets, true)
	read := func(shopID uuid.UUID) (shop.BillFacts, error) {
		var got shop.BillFacts
		err := f.a.Env.DB.Tx(context.Background(), db.Scope{UserID: uuid.New(), ShopID: shopID, Role: enum.RoleShopStaff}, func(ctx context.Context, tx *db.Tx) (err error) {
			got, err = svc.BillFacts(ctx, tx, shopID)
			return err
		})
		return got, err
	}
	got, err := read(id)
	if err != nil || got.MinOrderCents != 5000 || got.MaxLines != 7 || got.MinPlatformMarginCents != 300 || got.Location.Lat == 0 {
		t.Fatalf("facts = %+v, %v", got, err)
	}
	if _, err := read(randomID()); !apperr.Is(err, apperr.NotFound) {
		t.Errorf("unknown shop: %v, want NOT_FOUND", err)
	}
}
