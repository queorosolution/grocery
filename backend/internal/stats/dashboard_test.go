package stats_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"gp/internal/stats"
	"gp/test/apptest"
	"gp/test/fakes"
)

// scenario seeds two shops with known days (docs/06 section 8, "dashboard numbers
// match a seeded scenario"):
//
//	shop A: 26 Oct  sales 100000 (2 orders)   29 Oct  sales 50000 (1 order), one rejected
//	shop B: 27 Oct  sales 30000 (1 order)
type scenario struct {
	a      *apptest.App
	shopA  uuid.UUID
	shopB  uuid.UUID
	owner  apptest.Client
	staff  apptest.Client
	otherB apptest.Client
}

func seed(t *testing.T) scenario {
	t.Helper()
	a := apptest.New(t)
	s := scenario{a: a, shopA: a.SeedShop("alpha"), shopB: a.SeedShop("bravo")}
	row := func(shop uuid.UUID, day string, submitted, paid, delivered, rejected int, gmv, shopItems, income, discount int64, rev, pick int) {
		a.Exec(`INSERT INTO shop_daily_stats (shop_id, day, orders_submitted, orders_paid, orders_delivered, orders_rejected, gmv_cents, shop_items_cents,
			platform_income_cents, discount_cents, review_seconds_sum, review_count, pick_seconds_sum, pick_count)
			VALUES ($1, $2::date, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
			shop, day, submitted, paid, delivered, rejected, gmv, shopItems, income, discount, int64(rev*60), 1, int64(pick*60), 1)
	}
	row(s.shopA, "2026-10-26", 2, 2, 2, 0, 100000, 85000, 12000, 0, 2, 10)
	row(s.shopA, "2026-10-29", 2, 1, 1, 1, 50000, 42000, 6000, 1000, 4, 20)
	row(s.shopB, "2026-10-27", 1, 1, 1, 0, 30000, 26000, 3500, 0, 1, 5)
	s.owner = s.client("owner", s.shopA, "SHOP_OWNER")
	s.staff = s.client("staff", s.shopA, "SHOP_STAFF")
	s.otherB = s.client("ownerb", s.shopB, "SHOP_OWNER")
	return s
}

func (s scenario) client(sub string, shop uuid.UUID, role string) apptest.Client {
	c := s.a.As(s.a.SignedIn(fakes.Claims{Sub: sub, Email: sub + "@example.com", Name: sub}))
	s.a.Grant(c.Session.Me.ID, &shop, role)
	return c
}

func (s scenario) shopDash(c apptest.Client, shop uuid.UUID, query string) stats.ShopDashboard {
	return apptest.Decode[stats.ShopDashboard](s.a.T, c.OK(http.MethodGet, "/console/shops/"+shop.String()+"/dashboard"+query, ""))
}

// FR-DASH-01: the owner's numbers are the sums of the shop's days.
func TestDashboard_OwnerNumbers(t *testing.T) {
	s := seed(t)
	d := s.shopDash(s.owner, s.shopA, "?from=2026-10-25&to=2026-10-31")
	f := d.Full
	if f == nil {
		t.Fatal("no full part for the owner")
	}
	if f.SalesCents != 150000 || f.Orders != 3 || f.Delivered != 3 || f.AvgBillCents != 50000 || f.ShopItemsCents != 127000 || f.DiscountCents != 1000 {
		t.Errorf("full = %+v", f)
	}
	// 1 rejected of 4 submitted.
	if f.CancelRateBps != 2500 {
		t.Errorf("cancel rate = %d", f.CancelRateBps)
	}
	if len(f.Days) != 2 || f.Days[0].Day != "2026-10-26" || f.Days[0].SalesCents != 100000 || f.Days[1].Orders != 1 {
		t.Errorf("days = %+v", f.Days)
	}
	if d.From != "2026-10-25" || d.To != "2026-10-31" {
		t.Errorf("range = %s..%s", d.From, d.To)
	}
	// Review: (2 + 4) min over 2 answers; pick: (10 + 20) min over 2.
	if d.AvgReviewSeconds != 180 || d.AvgPickSeconds != 900 {
		t.Errorf("review %d, pick %d", d.AvgReviewSeconds, d.AvgPickSeconds)
	}
}

// A range with one day sees only that day; a day with no row is zeros.
func TestDashboard_RangeLimitsTheDays(t *testing.T) {
	s := seed(t)
	if f := s.shopDash(s.owner, s.shopA, "?from=2026-10-29&to=2026-10-29").Full; f.SalesCents != 50000 || f.Orders != 1 || len(f.Days) != 1 {
		t.Errorf("one day = %+v", f)
	}
	empty := s.shopDash(s.owner, s.shopA, "?from=2026-10-01&to=2026-10-02")
	if empty.Full.SalesCents != 0 || empty.Full.AvgBillCents != 0 || empty.Full.CancelRateBps != 0 || len(empty.Full.Days) != 0 || empty.AvgReviewSeconds != 0 {
		t.Errorf("empty = %+v", empty)
	}
}

// The staff have the queue and the speed, but no money (FR-DASH-02).
func TestDashboard_StaffSeeNoMoney(t *testing.T) {
	s := seed(t)
	d := s.shopDash(s.staff, s.shopA, "?from=2026-10-25&to=2026-10-31")
	if d.Full != nil {
		t.Fatalf("staff got %+v", d.Full)
	}
	if d.Queue == nil || d.AvgReviewSeconds != 180 {
		t.Errorf("ops = %+v", d.ShopOps)
	}
	raw := s.staff.OK(http.MethodGet, "/console/shops/"+s.shopA.String()+"/dashboard?from=2026-10-25&to=2026-10-31", "").Body.String()
	for _, word := range []string{"salesCents", "payoutDueCents", "marginCents", "topProducts"} {
		if strings.Contains(raw, word) {
			t.Errorf("the staff answer holds %q", word)
		}
	}
}

// CLAUDE.md rule: another shop's console is never readable, and the numbers never mix.
func TestDashboard_OtherShopIsNotVisible(t *testing.T) {
	s := seed(t)
	s.owner.Expect(http.MethodGet, "/console/shops/"+s.shopB.String()+"/dashboard", "", http.StatusNotFound, "NOT_FOUND")
	b := s.shopDash(s.otherB, s.shopB, "?from=2026-10-25&to=2026-10-31")
	if b.Full.SalesCents != 30000 || b.Full.Orders != 1 {
		t.Errorf("shop B = %+v", b.Full)
	}
}

func TestDashboard_BadRange(t *testing.T) {
	s := seed(t)
	for _, q := range []string{"?from=bad", "?to=2026-13-01", "?from=2026-10-30&to=2026-10-01", "?from=2024-01-01&to=2026-10-01"} {
		s.owner.Expect(http.MethodGet, "/console/shops/"+s.shopA.String()+"/dashboard"+q, "", http.StatusBadRequest, "VALIDATION_ERROR")
	}
}

// FR-DASH-04: the admin sees every shop.
func TestDashboard_Admin(t *testing.T) {
	s := seed(t)
	admin := s.a.AsAdmin()
	d := apptest.Decode[stats.AdminDashboard](t, admin.OK(http.MethodGet, "/admin/dashboard?from=2026-10-25&to=2026-10-31", ""))
	if d.GMVCents != 180000 || d.Orders != 4 || d.Delivered != 4 || d.AvgBillCents != 45000 || d.PlatformIncomeCents != 21500 {
		t.Errorf("totals = %+v", d)
	}
	if len(d.Shops) != 2 || d.Shops[0].Name != "Shop alpha" || d.Shops[0].SalesCents != 150000 || d.Shops[1].SalesCents != 30000 {
		t.Errorf("shops = %+v", d.Shops)
	}
	if len(d.Days) != 3 {
		t.Errorf("days = %+v", d.Days)
	}
	// Nothing was sent, failed or synced in this scenario; the counts are zero, not missing.
	if d.SMSSent != 0 || d.FailedPayments != 0 || d.Sync.OpenIssues != 0 || d.CouponCostCents != 0 {
		t.Errorf("platform numbers = %+v", d)
	}
	// Not for a shop's owner.
	s.owner.Expect(http.MethodGet, "/admin/dashboard", "", http.StatusForbidden, "FORBIDDEN")
}

// The admin's platform numbers come from the ledger, SMS, payments and syncs.
func TestDashboard_AdminPlatformNumbers(t *testing.T) {
	s := seed(t)
	a := s.a
	a.Exec(`INSERT INTO ledger_entries (shop_id, type, party, amount_cents) VALUES
		($1, 'PLATFORM_MARGIN', 'PLATFORM', 8000), ($1, 'SERVICE_FEE', 'PLATFORM', 2000), ($1, 'DISCOUNT', 'PLATFORM', -500), ($1, 'DISCOUNT', 'SHOP', -300)`, s.shopA)
	a.Exec(`INSERT INTO notifications (channel, template, to_masked, status, cost_units) VALUES ('SMS', 'order.paid', '+9477***1111', 'SENT', 1), ('SMS', 'order.paid', '+9477***2222', 'FAILED', 0)`)
	d := apptest.Decode[stats.AdminDashboard](t, a.AsAdmin().OK(http.MethodGet, "/admin/dashboard", ""))
	if d.MarginCents != 8000 || d.ServiceFeeCents != 2000 || d.CouponCostCents != 500 {
		t.Errorf("ledger numbers = %d %d %d", d.MarginCents, d.ServiceFeeCents, d.CouponCostCents)
	}
	if d.SMSSent != 1 {
		t.Errorf("sms sent = %d", d.SMSSent)
	}
}

// docs/05 section 7: the nightly job deletes only what is past its time.
func TestRollup_DeletesPastRetention(t *testing.T) {
	s := seed(t)
	a := s.a
	a.Exec(`INSERT INTO rate_counters (key, window_start, count) VALUES ('old', now() - interval '8 days', 1), ('new', now() - interval '1 day', 1)`)
	a.Exec(`UPDATE sessions SET expires_at = now() - interval '31 days' WHERE user_id = $1`, s.staff.Session.Me.ID)
	sessions := a.Count(`SELECT count(*) FROM sessions`)
	got, err := stats.NewService(a.Env.DB, nil).Rollup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got["rate_counters"] != 1 || a.Count(`SELECT count(*) FROM rate_counters WHERE key = 'new'`) != 1 {
		t.Errorf("rate_counters: %v", got)
	}
	if got["sessions"] < 1 || a.Count(`SELECT count(*) FROM sessions`) != sessions-int(got["sessions"]) || a.Count(`SELECT count(*) FROM sessions WHERE user_id = $1`, s.owner.Session.Me.ID) != 1 {
		t.Errorf("sessions: %v (had %d)", got, sessions)
	}
	// The stats and the finance records stay.
	if a.Count(`SELECT count(*) FROM shop_daily_stats`) != 3 {
		t.Error("the job removed stats rows")
	}
}
